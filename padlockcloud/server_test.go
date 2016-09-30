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
import "time"

const (
	testEmail = "martin@padlock.io"
	testData  = "Hello World!"
)

var (
	server  *Server
	client  *http.Client
	storage *MemoryStorage
	sender  *RecordSender
	host    string
)

func init() {
	storage = &MemoryStorage{}
	sender = &RecordSender{}
	templates := &Templates{
		template.Must(template.New("").Parse("{{ .token.Email }}, {{ .activation_link }}")),
		htmlTemplate.Must(htmlTemplate.New("").Parse("{{ .token.Email }}")),
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
	server = NewServer(logger, storage, sender, &ServerConfig{})
	server.Templates = templates
	server.Init()

	testServer := httptest.NewServer(server.HandlePanic(server.mux))

	host = testServer.URL

	jar, _ := cookiejar.New(nil)
	client = &http.Client{
		// CheckRedirect: func(req *http.Request, via []*http.Request) error {
		// 	return http.ErrUseLastResponse
		// },
		Jar: jar,
	}
}

func resetStorage() {
	storage.Open()
}

func resetCookies() {
	jar, _ := cookiejar.New(nil)
	client.Jar = jar
}

func resetAll() {
	resetCookies()
	resetStorage()
	sender.Reset()
}

func followRedirects(follow bool) {
	if follow {
		client.CheckRedirect = nil
	} else {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
}

// Helper function for creating (optionally authenticated) requests
func request(method string, url string, body string, authToken *AuthToken, version int) (*http.Response, error) {
	req, err := http.NewRequest(method, url, bytes.NewBuffer([]byte(body)))
	if err != nil {
		return nil, err
	}

	if method == "POST" || method == "PUT" {
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

	return client.Do(req)
}

// Helper function for checking a `http.Response` object for an expected status code and response body
// `body` is evaluated as a regular expression which the actual response body is matched against. If
// one wants to do a strict test against a specific string, the start and end entities should be used.
// E.g.:
// ```
// // Response body should be empty
// validateResponse(t, res, 204, "^$")
// ```
func validateResponse(res *http.Response, code int, body string) ([]byte, error) {
	if res.StatusCode != code {
		return nil, fmt.Errorf("%s %s: Expected status code to be %d, is %d", res.Request.Method, res.Request.URL, code, res.StatusCode)
	}

	defer res.Body.Close()
	resBody, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("Error: %v", err)
	}

	match, err := regexp.Match(body, resBody)
	if err != nil {
		log.Fatal(err)
	}

	if !match {
		return nil, fmt.Errorf("%s %s: Expected response body to match \"%s\", is \"%s\"", res.Request.Method, res.Request.URL, body, resBody)
	}

	return resBody, nil
}

func testResponse(t *testing.T, res *http.Response, code int, body string) bool {
	if _, err := validateResponse(res, code, body); err != nil {
		t.Error(err)
		return false
	}

	return true
}

func testError(t *testing.T, res *http.Response, e ErrorResponse) {
	testResponse(t, res, e.Status(), regexp.QuoteMeta(string(JsonifyErrorResponse(e))))
}

func extractActivationLink() (string, error) {
	// Activation message should be sent to the correct email
	if sender.Recipient != testEmail {
		return "", fmt.Errorf("Expected activation message to be sent to %s, instead got %s", testEmail, sender.Recipient)
	}

	// Activation message should contain a valid activation link
	linkPattern := fmt.Sprintf("%s/activate/\\?v=%d&t=%s", host, ApiVersion, tokenPattern)
	msgPattern := fmt.Sprintf("%s, %s", testEmail, linkPattern)
	match, _ := regexp.MatchString(msgPattern, sender.Message)
	if !match {
		return "", fmt.Errorf("Expected activation message to match \"%s\", got \"%s\"", msgPattern, sender.Message)
	}
	link := regexp.MustCompile(linkPattern).FindString(sender.Message)

	return link, nil
}

func TestAuthApi(t *testing.T) {
	var res *http.Response
	followRedirects(false)
	resetAll()

	// Using the PUT method for an account that hasn't been created yet should result in a 404
	res, _ = request("PUT", host+"/auth/", url.Values{
		"email": {testEmail},
	}.Encode(), nil, ApiVersion)
	testResponse(t, res, http.StatusNotFound, "")

	// POST means creating a new account is allowed
	res, _ = request("POST", host+"/auth/", url.Values{
		"email": {testEmail},
		"type":  {"api"},
	}.Encode(), nil, ApiVersion)
	responseBody, err := validateResponse(res, http.StatusAccepted, "")
	if err != nil {
		t.Fatal(err)
	}

	authToken := &AuthToken{}
	// Response status code should be "ACCEPTED", response body should be the json-encoded auth token
	if err := json.Unmarshal(responseBody, authToken); err != nil {
		t.Fatalf("Expected response to be JSON representation of api key, got %s", responseBody)
	}

	link, err := extractActivationLink()
	if err != nil {
		t.Fatal(err)
	}

	// 'visit' activation link
	res, _ = request("GET", link, "", nil, 0)

	testResponse(t, res, http.StatusOK, fmt.Sprintf("^%s$", testEmail))

	acc := &Account{Email: testEmail}
	storage.Get(acc)

	if !authToken.Validate(acc) {
		t.Error("Token should be activated")
	}

	if authToken.Type != "api" {
		t.Errorf("Wrong token type. Expected 'api', got '%s'", authToken.Type)
	}

	// For now, api auth tokens don't expire
	if !authToken.Expires.IsZero() {
		t.Errorf("Api auth tokens should not expire")
	}
}

func TestWebLogin(t *testing.T) {
	login := func(redirect string) (*AuthToken, *http.Response) {
		// POST means creating a new account is allowed
		res, _ := request("POST", host+"/auth/", url.Values{
			"email":    {testEmail},
			"type":     {"web"},
			"redirect": {redirect},
		}.Encode(), nil, ApiVersion)
		if _, err := validateResponse(res, http.StatusAccepted, ""); err != nil {
			return nil, res
		}

		link, err := extractActivationLink()
		if err != nil {
			t.Fatal(err)
		}

		// 'visit' activation link
		res, _ = request("GET", link, "", nil, 0)

		u, _ := url.Parse(host)
		cookies := client.Jar.Cookies(u)
		if len(cookies) != 1 || cookies[0].Name != "auth" {
			t.Fatalf("Expected cookie of name 'auth' to be set")
		}

		authToken, err := AuthTokenFromString(cookies[0].Value)
		if err != nil {
			t.Fatalf("Failed to parse auth token from cookie. Error: %v", err)
		}

		acc := &Account{Email: testEmail}
		storage.Get(acc)

		if !authToken.Validate(acc) {
			t.Error("Token should be activated")
		}

		if authToken.Type != "web" {
			t.Errorf("Wrong token type. Expected 'web', got '%s'", authToken.Type)
		}

		// For now, api auth tokens don't expire
		if !authToken.Expires.Before(time.Now().Add(time.Hour)) {
			t.Errorf("Web auth tokens should expire after at most an hour")
		}

		return authToken, res
	}

	// By default user should be redirected to dasboard after login
	_, res := login("")
	testResponse(t, res, http.StatusFound, "")
	if l := res.Header.Get("Location"); l != "/dashboard/" {
		t.Errorf("Expected redirect to %s, got %s", "/dashboard/")
	}

	// Redirect to other supported endpoints is also allowed
	_, res = login("/deletestore/")
	testResponse(t, res, http.StatusFound, "")
	if l := res.Header.Get("Location"); l != "/deletestore/" {
		t.Errorf("Expected redirect to %s, got %s", "/deletestore/")
	}

	// Using an external url or any unsupported endpoint should be treated as a bad request
	_, res = login("http://attacker.com")
	testError(t, res, &BadRequest{"invalid redirect path"})
	resetAll()
	_, res = login("/notsupported/")
	testError(t, res, &BadRequest{"invalid redirect path"})
}

// Full lifecycle test including
// - Requesting an api key
// - Activating an api key
// - Getting data
// - Putting data
// - Requesting a data reset
// - Confirming a data reset
func TestApiLifeCycle(t *testing.T) {
	followRedirects(true)

	resetCookies()
	resetStorage()

	// Post request for api key
	res, _ := request("POST", host+"/auth/", url.Values{
		"email": {testEmail},
	}.Encode(), nil, ApiVersion)

	// Response status code should be "ACCEPTED", response body should be the json-encoded auth token
	tokenJSON, err := validateResponse(res, http.StatusAccepted, "")
	if err != nil {
		t.Fatal(err)
	}

	authToken := &AuthToken{}
	if err := json.Unmarshal(tokenJSON, authToken); err != nil {
		t.Errorf("Expected response to be JSON representation of api key, got %s", tokenJSON)
	}

	// Activation message should be sent to the correct email
	if sender.Recipient != testEmail {
		t.Errorf("Expected activation message to be sent to %s, instead got %s", testEmail, sender.Recipient)
	}

	link, err := extractActivationLink()
	if err != nil {
		t.Fatal(err)
	}

	// 'visit' activation link
	res, _ = request("GET", link, "", nil, 0)
	testResponse(t, res, http.StatusOK, "")

	// Get data request authenticated with obtained api key should return with status code 200 - OK and
	// empty response body (since we haven't written any data yet)
	res, _ = request("GET", host+"/store/", "", authToken, ApiVersion)
	testResponse(t, res, http.StatusOK, "^$")

	// Put request should return with status code 204 - NO CONTENT
	res, _ = request("PUT", host+"/store/", testData, authToken, ApiVersion)
	testResponse(t, res, http.StatusNoContent, "")

	// Now get data request should return the data previously saved through PUT
	res, _ = request("GET", host+"/store/", "", authToken, ApiVersion)
	testResponse(t, res, http.StatusOK, fmt.Sprintf("^%s$", testData))

	// Send data reset request. Response should have status code 202 - ACCEPTED
	sender.Reset()
	res, _ = request("DELETE", host+"/store/", "", authToken, ApiVersion)
	testResponse(t, res, http.StatusAccepted, "")

	// Activation message should be sent to the correct email
	if sender.Recipient != testEmail {
		t.Fatalf("Expected confirm delete message to be sent to %s, instead got %s", testEmail, sender.Recipient)
	}

	if link, err = extractActivationLink(); err != nil {
		t.Fatal(err)
	}

	// 'visit' link, should log in and redirect to delete store form
	res, _ = request("GET", link, "", nil, 0)
	testResponse(t, res, http.StatusOK, fmt.Sprintf("^deletestore$"))
}

func TestWeb(t *testing.T) {
	resetCookies()
	resetStorage()

	// If not logged in, should redirect to login page
	res, _ := request("GET", host+"/dashboard/", "", nil, 0)
	testResponse(t, res, http.StatusOK, "^login,,$")

	// Post request for api key
	res, _ = request("POST", host+"/auth/", url.Values{
		"email": {testEmail},
		"type":  {"web"},
	}.Encode(), nil, ApiVersion)
	testResponse(t, res, http.StatusAccepted, fmt.Sprintf("^login,%s,true$", testEmail))

	// Activation message should be sent to the correct email
	if sender.Recipient != testEmail {
		t.Errorf("Expected activation message to be sent to %s, instead got %s", testEmail, sender.Recipient)
	}

	link, err := extractActivationLink()
	if err != nil {
		t.Fatal(err)
	}

	// 'visit' activation link
	res, _ = request("GET", link, "", nil, 0)
	testResponse(t, res, http.StatusOK, "")

	// We should be logged in now, so dashboard should render
	res, _ = request("GET", host+"/dashboard/", "", nil, 0)
	testResponse(t, res, http.StatusOK, "^dashboard$")

	// Log out
	res, _ = request("GET", host+"/logout/", "", nil, 0)
	testResponse(t, res, http.StatusOK, "")

	// If not logged in, should redirect to login page
	res, _ = request("GET", host+"/dashboard/", "", nil, 0)
	testResponse(t, res, http.StatusOK, "^login,,$")
}

// TODO: Test authentication
//
// func TestCsrfProtection() {
//
// 	acc := &Account{Email: testEmail}
// 	t := NewAuthToken(testEmail, "web")
// 	acc.AddAuthToken(t)
// 	server.Put(acc)
//
// 	client.Jar.SetCookies(testUrl, []*http.Cookie{
// 		{
// 			Name:  "auth",
// 			Value: t.String(),
// 		},
// 	})
//
// }

// TODO: Test revoke api key

// TODO: Test reset data

// Test correct handling of various error conditions
func TestErrorConditions(t *testing.T) {
	resetCookies()
	resetStorage()

	// Trying to get an api key for a non-existing account using the PUT method should result in a 404
	res, _ := request("PUT", host+"/auth/", url.Values{
		"email": {"hello@world.com"},
	}.Encode(), nil, ApiVersion)

	// No account with this email exists yet and we have not specified 'create=true' in our request
	testError(t, res, &AccountNotFound{})

	// A request without a valid authorization header should return with status code 401 - Unauthorized
	res, _ = request("GET", host+"/store/", "", nil, ApiVersion)
	testError(t, res, &InvalidAuthToken{})

	// Requests with unsupported HTTP methods should return with 405 - method not allowed
	res, _ = request("POST", host+"/store/", "", nil, ApiVersion)
	testError(t, res, &MethodNotAllowed{})

	// Requests to unsupported paths should return with 404 - not found
	res, _ = request("GET", host+"/invalidpath", "", nil, ApiVersion)
	testError(t, res, &UnsupportedEndpoint{})

	// An invalid activation token should result in a bad request response
	res, _ = request("GET", host+"/activate/?t=asdf", "", nil, ApiVersion)
	testError(t, res, &BadRequest{"invalid activation token"})
}

func TestOutdatedVersion(t *testing.T) {
	sender := server.Sender.(*RecordSender)

	sender.Reset()
	token, _ := token()
	req, _ := http.NewRequest("GET", host+"/", nil)
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("ApiKey %s:%s", testEmail, token))
	res, _ := client.Do(req)
	testError(t, res, &UnsupportedEndpoint{})
	if sender.Recipient != testEmail {
		t.Errorf("Expected outdated message to be sent to %s, instead got %s", testEmail, sender.Recipient)
	}

	sender.Reset()
	res, _ = request("POST", host+"/auth/", url.Values{
		"email": {testEmail},
	}.Encode(), nil, 0)
	testError(t, res, &UnsupportedApiVersion{0, ApiVersion})
	if sender.Recipient != testEmail {
		t.Errorf("Expected outdated message to be sent to %s, instead got %s", testEmail, sender.Recipient)
	}
}

func TestPanicRecovery(t *testing.T) {

	// Make sure the server recovers properly from runtime panics in handler functions
	server.mux.HandleFunc("/panic/", func(w http.ResponseWriter, r *http.Request) {
		panic("Everyone panic!!!")
	})
	res, _ := request("GET", host+"/panic/", "", nil, 1)
	testError(t, res, &ServerError{})
	server.mux.HandleFunc("/panic2/", func(w http.ResponseWriter, r *http.Request) {
		panic(errors.New("Everyone panic!!!"))
	})
	res, _ = request("GET", host+"/panic2/", "", nil, 1)
	testError(t, res, &ServerError{})
}

func TestErrorFormat(t *testing.T) {

	e := &UnsupportedEndpoint{}
	testErr := func(format string, expected []byte) {
		req, _ := http.NewRequest("GET", fmt.Sprintf("%s/invalidpath/?v=%d", host, ApiVersion), nil)
		if format != "" {
			req.Header.Add("Accept", format)
		}
		res, _ := client.Do(req)
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
