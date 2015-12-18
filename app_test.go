package main

import "testing"
import "os"
import "fmt"
import "text/template"
import htmlTemplate "html/template"
import "net/http"
import "net/http/httptest"
import "net/url"
import "log"
import "io/ioutil"
import "regexp"
import "bytes"

// Mock implementation of the `Sender` interface. Simply records arguments passed to the `Send` method
type RecordSender struct {
	Receiver string
	Subject  string
	Message  string
}

func (s *RecordSender) Send(rec string, subj string, message string) error {
	s.Receiver = rec
	s.Subject = subj
	s.Message = message
	return nil
}

func (s *RecordSender) Reset() {
	s.Receiver = ""
	s.Subject = ""
	s.Message = ""
}

var (
	app       *App
	server    *httptest.Server
	storage   *MemoryStorage
	sender    *RecordSender
	authToken *AuthToken
	testEmail = "martin@padlock.io"
	testData  = "Hello World!"
)

func TestMain(m *testing.M) {
	storage = &MemoryStorage{}
	sender = &RecordSender{}
	templates := &Templates{
		template.Must(template.New("").Parse("{{ .email }}, {{ .activation_link }}")),
		template.Must(template.New("").Parse("{{ .email }}, {{ .delete_link }}")),
		htmlTemplate.Must(htmlTemplate.New("").Parse("")),
		htmlTemplate.Must(htmlTemplate.New("").Parse("{{ .email }}")),
		template.Must(template.New("").Parse("")),
	}

	app = NewApp(storage, sender, templates, Config{RequireTLS: false})

	app.Storage.Open()
	defer app.Storage.Close()

	server = httptest.NewServer(app)

	os.Exit(m.Run())
}

// Helper function for creating (optionally authenticated) requests
func request(method string, path string, body string, authToken *AuthToken, version int) (*http.Response, error) {
	req, _ := http.NewRequest(method, server.URL+path, bytes.NewBuffer([]byte(body)))

	if method == "POST" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	if version > 0 {
		req.Header.Add("Accept", fmt.Sprintf("application/vnd.padlock;version=%d", version))
	}

	if authToken != nil {
		req.Header.Add("Authorization", fmt.Sprintf("AuthToken %s:%s", authToken.Email, authToken.Token))
	}
	return http.DefaultClient.Do(req)
}

// Helper function for checking a `http.Response` object for an expected status code and response body
// `body` is evaluated as a regular expression which the actual response body is matched against. If
// one wants to do a strict test against a specific string, the start and end entities should be used.
// E.g.:
// ```
// // Response body should be empty
// checkResponse(t, res, 204, "^$")
// ```
func checkResponse(t *testing.T, res *http.Response, code int, body string) []byte {
	if res.StatusCode != code {
		t.Errorf("%s %s: Expected status code to be %d, is %d", res.Request.Method, res.Request.URL, code, res.StatusCode)
	}

	defer res.Body.Close()
	resBody, err := ioutil.ReadAll(res.Body)
	if err != nil {
		t.Errorf("Error: %v", err)
	}

	match, err := regexp.Match(body, resBody)
	if err != nil {
		log.Fatal(err)
	}

	if !match {
		t.Errorf("%s %s: Expected response body to match \"%s\", is \"%s\"", res.Request.Method, res.Request.URL, body, resBody)
	}

	return resBody
}

// Full lifecycle test including
// - Requesting an api key
// - Activating an api key
// - Getting data
// - Putting data
// - Requesting a data reset
// - Confirming a data reset
func TestLifeCycle(t *testing.T) {
	// Post request for api key
	res, _ := request("POST", "/auth/", url.Values{
		"email": {testEmail},
	}.Encode(), nil, version)

	// No account with this email exists yet and we have not specified 'create=true' in our request
	// so the status code of th response should be "PRECONDITION FAILED"
	checkResponse(t, res, http.StatusPreconditionFailed, "")

	// Post request for api key
	res, _ = request("POST", "/auth/", url.Values{
		"email":  {testEmail},
		"create": {"true"},
	}.Encode(), nil, version)

	// Response status code should be "ACCEPTED", response body should be the RFC4122-compliant auth token
	token := checkResponse(t, res, http.StatusAccepted, uuidPattern)

	// Activation message should be sent to the correct email
	if sender.Receiver != testEmail {
		t.Errorf("Expected activation message to be sent to %s, instead got %s", testEmail, sender.Receiver)
	}

	// Activation message should contain a valid activation link
	linkPattern := fmt.Sprintf("%s/auth/\\?v=%d&t=%s", server.URL, version, uuidPattern)
	msgPattern := fmt.Sprintf("%s, %s", testEmail, linkPattern)
	match, _ := regexp.MatchString(msgPattern, sender.Message)
	if !match {
		t.Errorf("Expected activation message to match \"%s\", got \"%s\"", msgPattern, sender.Message)
	}
	link := regexp.MustCompile(linkPattern).FindString(sender.Message)

	// 'visit' activation link
	res, _ = http.Get(link)
	checkResponse(t, res, http.StatusOK, "")

	authToken = &AuthToken{
		Email: testEmail,
		Token: string(token),
	}

	// Get data request authenticated with obtained api key should return with status code 200 - OK and
	// empty response body (since we haven't written any data yet)
	res, _ = request("GET", "/store/", "", authToken, version)
	checkResponse(t, res, http.StatusOK, "^$")

	// Put request should return with status code 204 - NO CONTENT
	res, _ = request("PUT", "/store/", testData, authToken, version)
	checkResponse(t, res, http.StatusNoContent, "")

	// Now get data request should return the data previously save through PUT
	res, _ = request("GET", "/store/", "", authToken, version)
	checkResponse(t, res, http.StatusOK, fmt.Sprintf("^%s$", testData))

	// Send data reset request. Response should have status code 202 - ACCEPTED
	sender.Reset()
	res, _ = request("DELETE", "/store/", "", authToken, version)
	checkResponse(t, res, http.StatusAccepted, "")

	// Activation message should be sent to the correct email
	if sender.Receiver != testEmail {
		t.Errorf("Expected confirm delete message to be sent to %s, instead got %s", testEmail, sender.Receiver)
	}

	// Confirmation message should contain a valid confirmation link
	linkPattern = fmt.Sprintf("%s/deletestore/\\?v=%d&t=%s", server.URL, version, uuidPattern)
	msgPattern = fmt.Sprintf("%s, %s", testEmail, linkPattern)
	match, _ = regexp.MatchString(msgPattern, sender.Message)
	if !match {
		t.Errorf("Expected activation message to match \"%s\", got \"%s\"", msgPattern, sender.Message)
	}
	link = regexp.MustCompile(linkPattern).FindString(sender.Message)

	// 'visit' confirmation link
	res, _ = http.Get(link)
	checkResponse(t, res, http.StatusOK, fmt.Sprintf("^%s$", testEmail))

	// After data reset, data should be an empty string
	res, _ = request("GET", "/store/", "", authToken, version)
	checkResponse(t, res, http.StatusOK, "^$")
}

// Test correct handling of various error conditions
func TestErrorConditions(t *testing.T) {
	// A request without a valid authorization header should return with status code 401 - Unauthorized
	res, _ := request("GET", "/store/", "", nil, version)
	checkResponse(t, res, http.StatusUnauthorized, "")

	// Requests with unsupported HTTP methods should return with 405 - method not allowed
	res, _ = request("POST", "/store/", "", nil, version)
	checkResponse(t, res, http.StatusMethodNotAllowed, "")

	// Requests to unsupported paths should return with 404 - not found
	res, _ = request("GET", "/invalidpath", "", nil, version)
	checkResponse(t, res, http.StatusNotFound, "")

	// In case `RequireTLS` is set to true, requests via http should be rejected with status code 403 - forbidden
	app.RequireTLS = true
	res, _ = request("GET", "", "", nil, version)
	checkResponse(t, res, http.StatusForbidden, "")
	app.RequireTLS = false
}

func TestOutdatedVersion(t *testing.T) {
	sender.Reset()
	res, _ := request("GET", "/", "", authToken, 0)
	checkResponse(t, res, http.StatusNotAcceptable, "")
	if sender.Receiver != testEmail {
		t.Errorf("Expected outdated message to be sent to %s, instead got %s", testEmail, sender.Receiver)
	}

	sender.Reset()
	res, _ = request("POST", "/auth", url.Values{
		"email": {testEmail},
	}.Encode(), nil, 0)
	checkResponse(t, res, http.StatusNotAcceptable, "")
	if sender.Receiver != testEmail {
		t.Errorf("Expected outdated message to be sent to %s, instead got %s", testEmail, sender.Receiver)
	}

	sender.Reset()
	res, _ = request("DELETE", "/"+testEmail, "", nil, 0)
	checkResponse(t, res, http.StatusNotAcceptable, "")
	if sender.Receiver != testEmail {
		t.Errorf("Expected outdated message to be sent to %s, instead got %s", testEmail, sender.Receiver)
	}
}
