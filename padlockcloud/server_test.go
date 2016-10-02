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
import "github.com/gorilla/csrf"

const (
	testEmail = "martin@padlock.io"
	testData  = "Hello World!"
)

var (
	server    *Server
	client    *http.Client
	storage   *MemoryStorage
	sender    *RecordSender
	host      string
	authToken *AuthToken
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
		Jar: jar,
	}

	authMaxAge = func(authType string) time.Duration {
		switch authType {
		case "web":
			return time.Millisecond * 10
		default:
			return time.Duration(0)
		}
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
	authToken = nil
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
func request(method string, url string, body string, version int) (*http.Response, error) {
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

	if authToken != nil && authToken.Type == "api" {
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

func testResponse(t *testing.T, res *http.Response, code int, body string) {
	if _, err := validateResponse(res, code, body); err != nil {
		t.Error(err)
	}
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

func loginApi(email string) (*http.Response, error) {
	var res *http.Response
	var err error

	if res, err = request("POST", host+"/auth/", url.Values{
		"email": {email},
		"type":  {"api"},
	}.Encode(), ApiVersion); err != nil {
		return nil, err
	}

	responseBody, err := validateResponse(res, http.StatusAccepted, "")
	if err != nil {
		return res, err
	}

	authToken = &AuthToken{}
	// Response status code should be "ACCEPTED", response body should be the json-encoded auth token
	if err := json.Unmarshal(responseBody, authToken); err != nil {
		return res, fmt.Errorf("Failed to parse api key from response: %s", responseBody)
	}

	authToken.Type = "api"

	link, err := extractActivationLink()
	if err != nil {
		return res, err
	}

	// 'visit' activation link
	if res, err = request("GET", link, "", 0); err != nil {
		return res, err
	}

	_, err = validateResponse(res, http.StatusOK, fmt.Sprintf("^%s$", testEmail))
	return res, err
}

func loginWeb(email string, redirect string) (*http.Response, error) {
	var res *http.Response
	var err error

	if res, err = request("POST", host+"/auth/", url.Values{
		"email":    {email},
		"type":     {"web"},
		"redirect": {redirect},
	}.Encode(), ApiVersion); err != nil {
		return nil, err
	}

	if _, err := validateResponse(res, http.StatusAccepted, ""); err != nil {
		return res, err
	}

	link, err := extractActivationLink()
	if err != nil {
		return res, err
	}

	// 'visit' activation link
	if res, err = request("GET", link, "", 0); err != nil {
		return res, err
	}

	u, _ := url.Parse(host)
	var authCookie *http.Cookie
	for _, c := range client.Jar.Cookies(u) {
		if c.Name == "auth" {
			authCookie = c
			break
		}
	}

	if authCookie == nil {
		return res, errors.New("Expected cookie of name 'auth' to be set")
	}

	if authToken, err = AuthTokenFromString(authCookie.Value); err != nil {
		return res, fmt.Errorf("Failed to parse auth token from cookie. Error: %v", err)
	}

	authToken.Type = "web"

	return res, nil
}

func TestAuthentication(t *testing.T) {
	var res *http.Response
	var err error
	var at *AuthToken
	var captureAuthToken = func(w http.ResponseWriter, r *http.Request, auth *AuthToken) error {
		at = auth
		return nil
	}

	followRedirects(false)

	server.Route(&Endpoint{
		Path:     "/authtestnoauth/",
		AuthType: "",
		Handlers: MethodFuncs{
			"GET": captureAuthToken,
		},
	})

	server.Route(&Endpoint{
		Path:     "/authtestapi/",
		AuthType: "api",
		Handlers: MethodFuncs{
			"GET": captureAuthToken,
		},
	})

	server.Route(&Endpoint{
		Path:     "/authtestweb/",
		AuthType: "web",
		Handlers: MethodFuncs{
			"GET": captureAuthToken,
		},
	})

	t.Run("account not found", func(t *testing.T) {
		resetAll()

		// Trying to get an api key for a non-existing account using the PUT method should result in a 404
		if res, _ := request("PUT", host+"/auth/", url.Values{
			"email": {"hello@world.com"},
		}.Encode(), ApiVersion); err != nil {
			t.Fatal(err)
		}
		// No account with this email exists yet and we have not specified 'create=true' in our request
		testError(t, res, &AccountNotFound{})
	})

	t.Run("unauthenticated", func(t *testing.T) {
		resetAll()
		at = nil

		// Request should go through for route not requiring authentication
		if res, err = request("GET", host+"/authtestnoauth/", "", 0); err != nil {
			t.Fatal(err)
		}
		testResponse(t, res, http.StatusOK, "")

		if at != nil {
			t.Error("Auth token to be passed to handler even though none was expected")
		}

		// Not logged in so we should be redirected to the login page
		if res, err = request("GET", host+"/authtestweb/", "", 0); err != nil {
			t.Fatal(err)
		}
		testResponse(t, res, http.StatusFound, "")
		if res.Header.Get("Location") != "/login/" {
			t.Error("Expected to be redirected to login page")
		}

		// Not authenticated so we should get a 401
		if res, err = request("GET", host+"/authtestapi/", "", 0); err != nil {
			t.Fatal(err)
		}
		testError(t, res, &InvalidAuthToken{})
	})

	t.Run("type=api", func(t *testing.T) {
		resetAll()
		at = nil

		if _, err = loginApi(testEmail); err != nil {
			t.Fatal(err)
		}

		// Route without authentication should always go through
		if res, err = request("GET", host+"/authtestnoauth/", "", 0); err != nil {
			t.Fatal(err)
		}
		testResponse(t, res, http.StatusOK, "")

		// Even though no authentication is required for this route, an auth token should still be
		// passed to the handler
		if at == nil {
			t.Error("No auth token passed to handler")
		} else {
			if at.Type != "api" {
				t.Error("Wrong token type. Expected %s, got %s", "api", at.Type)
			}

			if at.Email != testEmail {
				t.Errorf("Wrong account. Expected %s, got %s", testEmail, at.Email)
			}
		}

		// We're authenticated but with the wrong token type. Should get a 401
		if res, err = request("GET", host+"/authtestweb/", "", 0); err != nil {
			t.Fatal(err)
		}
		testError(t, res, &InvalidAuthToken{})

		// We're authenticated so the request should go through
		if res, err = request("GET", host+"/authtestapi/", "", 0); err != nil {
			t.Fatal(err)
		}
		testResponse(t, res, http.StatusOK, "")
	})

	t.Run("type=web", func(t *testing.T) {
		resetAll()
		at = nil

		if res, err = loginWeb(testEmail, ""); err != nil {
			t.Fatal(err)
		}

		// By default user should be redirected to dasboard after login
		testResponse(t, res, http.StatusFound, "")
		if l := res.Header.Get("Location"); l != "/dashboard/" {
			t.Errorf("Expected redirect to %s, got %s", "/dashboard/", res.Header.Get("Location"))
		}

		// Test route without authentication
		if res, err = request("GET", host+"/authtestnoauth/", "", 0); err != nil {
			t.Fatal(err)
		}
		testResponse(t, res, http.StatusOK, "")

		// Even though no authentication is required for this route, an auth token should still be
		// passed to the handler
		if at == nil {
			t.Error("No auth token passed to handler")
		} else {
			if at.Type != "web" {
				t.Error("Wrong token type. Expected %s, got %s", "web", at.Type)
			}

			if at.Email != testEmail {
				t.Errorf("Wrong account. Expected %s, got %s", testEmail, at.Email)
			}
		}

		// We're logged in, but with the wrong auth type. should get a 401
		if res, err = request("GET", host+"/authtestapi/", "", 0); err != nil {
			t.Fatal(err)
		}
		testError(t, res, &InvalidAuthToken{})

		// We're logged in so the request should go through
		if res, err = request("GET", host+"/authtestweb/", "", 0); err != nil {
			t.Fatal(err)
		}
		testResponse(t, res, http.StatusOK, "")

		// Make sure auth token expires
		time.Sleep(authMaxAge("web"))
		if res, err = request("GET", host+"/authtestweb/", "", 0); err != nil {
			t.Fatal(err)
		}
		testResponse(t, res, http.StatusFound, "")
		if res.Header.Get("Location") != "/login/" {
			t.Error("Expected to be redirected to login page")
		}

		// Redirect to other supported endpoints is also allowed
		if res, err = loginWeb(testEmail, "/deletestore/"); err != nil {
			t.Fatal(err)
		}
		testResponse(t, res, http.StatusFound, "")
		if l := res.Header.Get("Location"); l != "/deletestore/" {
			t.Errorf("Expected redirect to %s, got %s", "/deletestore/", res.Header.Get("Location"))
		}

		// Using an external url or any unsupported endpoint should be treated as a bad request
		res, err = loginWeb(testEmail, "http://attacker.com")
		testError(t, res, &BadRequest{"invalid redirect path"})

		res, err = loginWeb(testEmail, "/notsupported/")
		testError(t, res, &BadRequest{"invalid redirect path"})
	})

	t.Run("invalid activation token", func(t *testing.T) {
		// An invalid activation token should result in a bad request response
		res, _ = request("GET", host+"/activate/?t=asdf", "", ApiVersion)
		testError(t, res, &BadRequest{"invalid activation token"})
	})
}

func TestCsrfProtection(t *testing.T) {
	var res *http.Response
	var err error

	resetAll()
	followRedirects(true)

	server.Route(&Endpoint{
		Path:     "/csrftest/",
		AuthType: "web",
		Handlers: MethodFuncs{
			"GET": func(w http.ResponseWriter, r *http.Request, auth *AuthToken) error {
				w.Write([]byte(csrf.Token(r)))
				return nil
			},
			"POST": func(w http.ResponseWriter, r *http.Request, auth *AuthToken) error {
				return nil
			},
		},
	})

	if res, err = loginWeb(testEmail, "/csrftest/"); err != nil {
		t.Fatal(err)
	}

	csrfToken, err := validateResponse(res, http.StatusOK, "")
	if err != nil {
		t.Fatal(err)
	}

	if res, err = request("POST", host+"/csrftest/", url.Values{
		"gorilla.csrf.Token": {"asdf"},
	}.Encode(), ApiVersion); err != nil {
		t.Fatal(err)
	}
	testError(t, res, &InvalidCsrfToken{})

	if res, err = request("POST", host+"/csrftest/", url.Values{
		"gorilla.csrf.Token": {string(csrfToken)},
	}.Encode(), ApiVersion); err != nil {
		t.Fatal(err)
	}
	testResponse(t, res, http.StatusOK, "")
}

func TestStore(t *testing.T) {
	var res *http.Response
	var err error

	resetAll()
	followRedirects(true)

	t.Run("read(unauthenticated)", func(t *testing.T) {
		// Get data request authenticated with obtained api key should return with status code 200 - OK and
		// empty response body (since we haven't written any data yet)
		if res, err = request("GET", host+"/store/", "", ApiVersion); err != nil {
			t.Fatal(err)
		}
		testError(t, res, &InvalidAuthToken{})
	})

	if _, err := loginApi(testEmail); err != nil {
		t.Fatal(err)
	}

	t.Run("read(empty)", func(t *testing.T) {
		// Get data request authenticated with obtained api key should return with status code 200 - OK and
		// empty response body (since we haven't written any data yet)
		if res, err = request("GET", host+"/store/", "", ApiVersion); err != nil {
			t.Fatal(err)
		}
		testResponse(t, res, http.StatusOK, "^$")
	})

	t.Run("write", func(t *testing.T) {
		// Put request should return with status code 204 - NO CONTENT
		if res, err = request("PUT", host+"/store/", testData, ApiVersion); err != nil {
			t.Fatal(err)
		}
		testResponse(t, res, http.StatusNoContent, "")
	})

	t.Run("read(non-empty)", func(t *testing.T) {
		// Now get data request should return the data previously saved through PUT
		if res, err = request("GET", host+"/store/", "", ApiVersion); err != nil {
			t.Fatal(err)
		}
		testResponse(t, res, http.StatusOK, fmt.Sprintf("^%s$", testData))
	})

	t.Run("request delete", func(t *testing.T) {
		// Send data reset request. Response should have status code 202 - ACCEPTED
		if res, err = request("DELETE", host+"/store/", "", ApiVersion); err != nil {
			t.Fatal(err)
		}
		testResponse(t, res, http.StatusAccepted, "")

		// Activation message should be sent to the correct email
		if sender.Recipient != testEmail {
			t.Fatalf("Expected confirm delete message to be sent to %s, instead got %s", testEmail, sender.Recipient)
		}

		link, err := extractActivationLink()
		if err != nil {
			t.Fatal(err)
		}

		// 'visit' link, should log in and redirect to delete store form
		if res, err = request("GET", link, "", 0); err != nil {
			t.Fatal(err)
		}
		testResponse(t, res, http.StatusOK, fmt.Sprintf("^deletestore$"))
	})
}

func TestWeb(t *testing.T) {
	resetAll()
	followRedirects(true)

	// If not logged in, should redirect to login page
	res, _ := request("GET", host+"/dashboard/", "", 0)
	testResponse(t, res, http.StatusOK, "^login,,$")

	if _, err := loginWeb(testEmail, ""); err != nil {
		t.Fatal(err)
	}

	// We should be logged in now, so dashboard should render
	res, _ = request("GET", host+"/dashboard/", "", 0)
	testResponse(t, res, http.StatusOK, "^dashboard$")

	// Log out
	res, _ = request("GET", host+"/logout/", "", 0)
	testResponse(t, res, http.StatusOK, "")

	// If not logged in, should redirect to login page
	res, _ = request("GET", host+"/dashboard/", "", 0)
	testResponse(t, res, http.StatusOK, "^login,,$")
}

// TODO: Test revoke api key

// TODO: Test reset data

func TestMethodNotAllowed(t *testing.T) {
	// Requests with unsupported HTTP methods should return with 405 - method not allowed
	res, _ = request("POST", host+"/store/", "", ApiVersion)
	testError(t, res, &MethodNotAllowed{})
}

func TestUnsupportedEndpoint(t *testing.T) {
	// Requests to unsupported paths should return with 404 - not found
	res, _ = request("GET", host+"/invalidpath", "", ApiVersion)
	testError(t, res, &UnsupportedEndpoint{})
}

func TestOutdatedVersion(t *testing.T) {
	resetAll()

	// The root path is a special case in that the only way to figure out if the client is using
	// and older api version is if the Authorization header is using the 'ApiKey' authentication scheme
	token, _ := token()
	req, _ := http.NewRequest("GET", host+"/", nil)
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("ApiKey %s:%s", testEmail, token))
	res, _ := client.Do(req)
	testError(t, res, &UnsupportedEndpoint{})
	if sender.Recipient != testEmail {
		t.Errorf("Expected outdated message to be sent to %s, instead got %s", testEmail, sender.Recipient)
	}

	resetAll()

	// When doing an auth request, the email form field should be used for sending the notification since
	// the user is not authenticated
	res, _ = request("POST", host+"/auth/", url.Values{
		"email": {testEmail},
	}.Encode(), 0)
	testError(t, res, &UnsupportedApiVersion{0, ApiVersion})
	if sender.Recipient != testEmail {
		t.Errorf("Expected outdated message to be sent to %s, instead got %s", testEmail, sender.Recipient)
	}

	resetAll()

	loginApi(testEmail)

	// When doing an auth request, the email form field should be used for sending the notification since
	// the user is not authenticated
	res, _ = request("GET", host+"/store/", url.Values{
		"email": {testEmail},
	}.Encode(), 0)
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
	res, _ := request("GET", host+"/panic/", "", 1)
	testError(t, res, &ServerError{})
	server.mux.HandleFunc("/panic2/", func(w http.ResponseWriter, r *http.Request) {
		panic(errors.New("Everyone panic!!!"))
	})
	res, _ = request("GET", host+"/panic2/", "", 1)
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
