package padlockcloud

import "testing"
import "fmt"
import "text/template"
import htmlTemplate "html/template"
import "net/http"
import "net/http/cookiejar"
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

var testClient *http.Client

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
		req.Header.Add("Authorization", authToken.String())
	}

	return testClient.Do(req)
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
		template.Must(template.New("").Parse("{{ .token.Email }}, {{ .activation_link }}")),
		htmlTemplate.Must(htmlTemplate.New("").Parse("")),
		template.Must(template.New("").Parse("")),
		htmlTemplate.Must(htmlTemplate.New("").Parse("<html>{{ .message }}</html>")),
		htmlTemplate.Must(htmlTemplate.New("").Parse("login,{{ .email }},{{ .submitted }}")),
		htmlTemplate.Must(htmlTemplate.New("").Parse("dashboard")),
		htmlTemplate.Must(htmlTemplate.New("").Parse("deletestore")),
	}

	logger := &Log{Config: &LogConfig{}}
	logger.Init()
	logger.Info.SetOutput(ioutil.Discard)
	logger.Error.SetOutput(ioutil.Discard)
	server := NewServer(logger, storage, sender, &ServerConfig{})
	server.Templates = templates
	server.Init()

	testServer := httptest.NewServer(server.HandlePanic(server.mux))

	return server, testServer.URL
}

// Full lifecycle test including
// - Requesting an api key
// - Activating an api key
// - Getting data
// - Putting data
// - Requesting a data reset
// - Confirming a data reset
func TestApiLifeCycle(t *testing.T) {
	server, testURL := setupServer()
	sender := server.Sender.(*RecordSender)

	// Post request for api key
	res, _ := request("POST", testURL+"/auth/", url.Values{
		"email": {testEmail},
	}.Encode(), true, nil, ApiVersion)

	// Response status code should be "ACCEPTED", response body should be the json-encoded auth token
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
	res, _ = request("GET", link, "", false, nil, 0)
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
		t.Fatalf("Expected confirm delete message to be sent to %s, instead got %s", testEmail, sender.Recipient)
	}

	// Confirmation message should contain a valid confirmation link
	linkPattern = fmt.Sprintf("%s/activate/\\?v=%d&t=%s", testURL, ApiVersion, tokenPattern)
	msgPattern = fmt.Sprintf("%s, %s", testEmail, linkPattern)
	match, _ = regexp.MatchString(msgPattern, sender.Message)
	if !match {
		t.Fatalf("Expected activation message to match \"%s\", got \"%s\"", msgPattern, sender.Message)
	}
	link = regexp.MustCompile(linkPattern).FindString(sender.Message)

	// 'visit' link, should log in and redirect to delete store form
	res, _ = request("GET", link, "", false, nil, 0)
	testResponse(t, res, http.StatusOK, fmt.Sprintf("^deletestore$"))
}

func TestWebLogin(t *testing.T) {
	server, testURL := setupServer()
	sender := server.Sender.(*RecordSender)

	// If not logged in, should redirect to login page
	res, _ := request("GET", testURL+"/dashboard/", "", true, nil, 0)
	testResponse(t, res, http.StatusOK, "^login,,$")

	// Post request for api key
	res, _ = request("POST", testURL+"/auth/", url.Values{
		"email": {testEmail},
		"type":  {"web"},
	}.Encode(), true, nil, ApiVersion)
	testResponse(t, res, http.StatusAccepted, fmt.Sprintf("^login,%s,true$", testEmail))

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
	res, _ = request("GET", link, "", false, nil, 0)
	testResponse(t, res, http.StatusOK, "")

	// We should be logged in now, so dashboard should render
	res, _ = request("GET", testURL+"/dashboard/", "", true, nil, 0)
	testResponse(t, res, http.StatusOK, "^dashboard$")

	// Log out
	res, _ = request("GET", testURL+"/logout/", "", true, nil, 0)
	testResponse(t, res, http.StatusOK, "")

	// If not logged in, should redirect to login page
	res, _ = request("GET", testURL+"/dashboard/", "", true, nil, 0)
	testResponse(t, res, http.StatusOK, "^login,,$")
}

// Test correct handling of various error conditions
func TestErrorConditions(t *testing.T) {
	_, testURL := setupServer()

	// Trying to get an api key for a non-existing account using the PUT method should result in a 404
	res, _ := request("PUT", testURL+"/auth/", url.Values{
		"email": {"hello@world.com"},
	}.Encode(), true, nil, ApiVersion)

	// No account with this email exists yet and we have not specified 'create=true' in our request
	testError(t, res, &AccountNotFound{})

	// A request without a valid authorization header should return with status code 401 - Unauthorized
	res, _ = request("GET", testURL+"/store/", "", false, nil, ApiVersion)
	testError(t, res, &InvalidAuthToken{})

	// Requests with unsupported HTTP methods should return with 405 - method not allowed
	res, _ = request("POST", testURL+"/store/", "", false, nil, ApiVersion)
	testError(t, res, &MethodNotAllowed{})

	// Requests to unsupported paths should return with 404 - not found
	res, _ = request("GET", testURL+"/invalidpath", "", false, nil, ApiVersion)
	testError(t, res, &UnsupportedEndpoint{})

	// An invalid activation token should result in a bad request response
	res, _ = request("GET", testURL+"/activate/?t=asdf", "", false, nil, ApiVersion)
	testError(t, res, &BadRequest{"invalid activation token"})
}

func TestOutdatedVersion(t *testing.T) {
	server, testURL := setupServer()
	sender := server.Sender.(*RecordSender)

	sender.Reset()
	token, _ := token()
	req, _ := http.NewRequest("GET", testURL+"/", nil)
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("ApiKey %s:%s", testEmail, token))
	res, _ := testClient.Do(req)
	testError(t, res, &UnsupportedEndpoint{})
	if sender.Recipient != testEmail {
		t.Errorf("Expected outdated message to be sent to %s, instead got %s", testEmail, sender.Recipient)
	}

	sender.Reset()
	res, _ = request("POST", testURL+"/auth/", url.Values{
		"email": {testEmail},
	}.Encode(), true, nil, 0)
	testError(t, res, &UnsupportedApiVersion{0, ApiVersion})
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
		res, _ := testClient.Do(req)
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

func init() {
	jar, _ := cookiejar.New(nil)
	testClient = &http.Client{
		// CheckRedirect: func(req *http.Request, via []*http.Request) error {
		// 	return http.ErrUseLastResponse
		// },
		Jar: jar,
	}
}
