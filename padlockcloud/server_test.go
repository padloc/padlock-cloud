package padlockcloud

import "testing"
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
import "encoding/json"
import "errors"

const (
	testEmail = "martin@padlock.io"
	testData  = "Hello World!"
)

// Helper function for creating (optionally authenticated) requests
func request(method string, url string, body string, asForm bool, authToken *AuthToken, version int) (*http.Response, error) {
	req, _ := http.NewRequest(method, url, bytes.NewBuffer([]byte(body)))

	if asForm {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	if version > 0 {
		req.Header.Add("Accept", fmt.Sprintf("application/vnd.padlock;version=%d", version))
	} else {
		req.Header.Add("Accept", "application/json")
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
// testResponse(t, res, 204, "^$")
// ```
func testResponse(t *testing.T, res *http.Response, code int, body string) []byte {
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

func testError(t *testing.T, res *http.Response, e ErrorResponse) {
	testResponse(t, res, e.Status(), regexp.QuoteMeta(string(JsonifyErrorResponse(e))))
}

func setupServer() (*Server, string) {
	storage := &MemoryStorage{}
	sender := &RecordSender{}
	templates := &Templates{
		template.Must(template.New("").Parse("{{ .email }}, {{ .activation_link }}")),
		template.Must(template.New("").Parse("{{ .email }}, {{ .delete_link }}")),
		htmlTemplate.Must(htmlTemplate.New("").Parse("")),
		htmlTemplate.Must(htmlTemplate.New("").Parse("{{ .email }}")),
		template.Must(template.New("").Parse("")),
		htmlTemplate.Must(htmlTemplate.New("").Parse("<html>{{ .message }}</html>")),
	}

	logger := &Log{Config: &LogConfig{}}
	logger.Init()
	logger.Info.SetOutput(ioutil.Discard)
	logger.Error.SetOutput(ioutil.Discard)
	server := NewServer(logger, storage, sender, &ServerConfig{RequireTLS: false})
	server.Templates = templates
	server.Init()

	testServer := httptest.NewServer(server)

	return server, testServer.URL
}

// Full lifecycle test including
// - Requesting an api key
// - Activating an api key
// - Getting data
// - Putting data
// - Requesting a data reset
// - Confirming a data reset
func TestLifeCycle(t *testing.T) {
	server, testURL := setupServer()
	sender := server.Sender.(*RecordSender)

	// Post request for api key
	res, _ := request("POST", testURL+"/auth/", url.Values{
		"email": {testEmail},
	}.Encode(), true, nil, ApiVersion)

	// Response status code should be "ACCEPTED", response body should be the RFC4122-compliant auth token
	tokenJSON := testResponse(t, res, http.StatusAccepted, "")

	authToken := &AuthToken{}
	err := json.Unmarshal(tokenJSON, authToken)

	if err != nil {
		t.Errorf("Expected response to be JSON representation of api key, got %s", tokenJSON)
	}

	// Activation message should be sent to the correct email
	if sender.Recipient != testEmail {
		t.Errorf("Expected activation message to be sent to %s, instead got %s", testEmail, sender.Recipient)
	}

	// Activation message should contain a valid activation link
	linkPattern := fmt.Sprintf("%s/activate/\\?v=%d&t=%s", testURL, ApiVersion, tokenPattern)
	msgPattern := fmt.Sprintf("%s, %s", testEmail, linkPattern)
	match, _ := regexp.MatchString(msgPattern, sender.Message)
	if !match {
		t.Errorf("Expected activation message to match \"%s\", got \"%s\"", msgPattern, sender.Message)
	}
	link := regexp.MustCompile(linkPattern).FindString(sender.Message)

	// 'visit' activation link
	res, _ = http.Get(link)
	testResponse(t, res, http.StatusOK, "")

	// Get data request authenticated with obtained api key should return with status code 200 - OK and
	// empty response body (since we haven't written any data yet)
	res, _ = request("GET", testURL+"/store/", "", false, authToken, ApiVersion)
	testResponse(t, res, http.StatusOK, "^$")

	// Put request should return with status code 204 - NO CONTENT
	res, _ = request("PUT", testURL+"/store/", testData, false, authToken, ApiVersion)
	testResponse(t, res, http.StatusNoContent, "")

	// Now get data request should return the data previously saved through PUT
	res, _ = request("GET", testURL+"/store/", "", false, authToken, ApiVersion)
	testResponse(t, res, http.StatusOK, fmt.Sprintf("^%s$", testData))

	// Send data reset request. Response should have status code 202 - ACCEPTED
	sender.Reset()
	res, _ = request("DELETE", testURL+"/store/", "", false, authToken, ApiVersion)
	testResponse(t, res, http.StatusAccepted, "")

	// Activation message should be sent to the correct email
	if sender.Recipient != testEmail {
		t.Errorf("Expected confirm delete message to be sent to %s, instead got %s", testEmail, sender.Recipient)
	}

	// Confirmation message should contain a valid confirmation link
	linkPattern = fmt.Sprintf("%s/deletestore/\\?v=%d&t=%s", testURL, ApiVersion, tokenPattern)
	msgPattern = fmt.Sprintf("%s, %s", testEmail, linkPattern)
	match, _ = regexp.MatchString(msgPattern, sender.Message)
	if !match {
		t.Errorf("Expected activation message to match \"%s\", got \"%s\"", msgPattern, sender.Message)
	}
	link = regexp.MustCompile(linkPattern).FindString(sender.Message)

	// 'visit' confirmation link
	res, _ = http.Get(link)
	testResponse(t, res, http.StatusOK, fmt.Sprintf("^%s$", testEmail))

	// After data reset, data should be an empty string
	res, _ = request("GET", testURL+"/store/", "", false, authToken, ApiVersion)
	testResponse(t, res, http.StatusOK, "^$")
}

// Test correct handling of various error conditions
func TestErrorConditions(t *testing.T) {
	server, testURL := setupServer()

	// Trying to get an api key for a non-existing account using the PUT method should result in a 404
	res, _ := request("PUT", testURL+"/auth/", url.Values{
		"email": {"hello@world.com"},
	}.Encode(), true, nil, ApiVersion)

	// No account with this email exists yet and we have not specified 'create=true' in our request
	testError(t, res, &AccountNotFound{})

	// A request without a valid authorization header should return with status code 401 - Unauthorized
	res, _ = request("GET", testURL+"/store/", "", false, nil, ApiVersion)
	testError(t, res, &Unauthorized{})

	// Requests with unsupported HTTP methods should return with 405 - method not allowed
	res, _ = request("POST", testURL+"/store/", "", false, nil, ApiVersion)
	testError(t, res, &MethodNotAllowed{})

	// Requests to unsupported paths should return with 404 - not found
	res, _ = request("GET", testURL+"/invalidpath", "", false, nil, ApiVersion)
	testError(t, res, &UnsupportedEndpoint{})

	// An invalid activation token should result in a bad request response
	res, _ = request("GET", testURL+"/activate/?t=asdf", "", false, nil, ApiVersion)
	testError(t, res, &InvalidToken{})

	// An invalid deletion token should result in a bad request response
	res, _ = request("GET", testURL+"/deletestore/?t=asdf", "", false, nil, ApiVersion)
	testError(t, res, &InvalidToken{})

	// In case `RequireTLS` is set to true, requests via http should be rejected with status code 403 - forbidden
	server.Config.RequireTLS = true
	res, _ = request("GET", testURL+"", "", false, nil, ApiVersion)
	testError(t, res, &InsecureConnection{})
	server.Config.RequireTLS = false
}

func TestOutdatedVersion(t *testing.T) {
	server, testURL := setupServer()
	sender := server.Sender.(*RecordSender)

	e := &UnsupportedApiVersion{}

	sender.Reset()
	token, _ := token()
	res, _ := request("GET", testURL+"/", "", false, &AuthToken{Email: testEmail, Token: token}, 0)
	testError(t, res, e)
	if sender.Recipient != testEmail {
		t.Errorf("Expected outdated message to be sent to %s, instead got %s", testEmail, sender.Recipient)
	}

	sender.Reset()
	res, _ = request("POST", testURL+"/auth", url.Values{
		"email": {testEmail},
	}.Encode(), true, nil, 0)
	testError(t, res, e)
	if sender.Recipient != testEmail {
		t.Errorf("Expected outdated message to be sent to %s, instead got %s", testEmail, sender.Recipient)
	}

	sender.Reset()
	res, _ = request("DELETE", testURL+"/"+testEmail, "", false, nil, 0)
	testError(t, res, e)
	if sender.Recipient != testEmail {
		t.Errorf("Expected outdated message to be sent to %s, instead got %s", testEmail, sender.Recipient)
	}
}

func TestPanicRecovery(t *testing.T) {
	server, testURL := setupServer()

	// Make sure the server recovers properly from runtime panics in handler functions
	server.mux.HandleFunc("/panic/", func(w http.ResponseWriter, r *http.Request) {
		panic("Everyone panic!!!")
	})
	res, _ := request("GET", testURL+"/panic/", "", false, nil, 1)
	testError(t, res, &ServerError{})
	server.mux.HandleFunc("/panic2/", func(w http.ResponseWriter, r *http.Request) {
		panic(errors.New("Everyone panic!!!"))
	})
	res, _ = request("GET", testURL+"/panic2/", "", false, nil, 1)
	testError(t, res, &ServerError{})
}

func TestErrorFormat(t *testing.T) {
	_, testURL := setupServer()

	e := &UnsupportedEndpoint{}
	testErr := func(format string, expected []byte) {
		req, _ := http.NewRequest("GET", fmt.Sprintf("%s/invalidpath/?v=%d", testURL, ApiVersion), nil)
		if format != "" {
			req.Header.Add("Accept", format)
		}
		res, _ := http.DefaultClient.Do(req)
		defer res.Body.Close()
		body, _ := ioutil.ReadAll(res.Body)
		if !bytes.Equal(body, expected) {
			t.Errorf("Expected %s, instead got %s", expected, body)
		}
	}

	testErr(fmt.Sprintf("application/vnd.padlock;version=%d", ApiVersion), JsonifyErrorResponse(e))
	testErr("application/json", JsonifyErrorResponse(e))
	testErr("text/html", []byte(fmt.Sprintf("<html>%s</html>", e.Message())))
	testErr("", []byte(e.Message()))
}
