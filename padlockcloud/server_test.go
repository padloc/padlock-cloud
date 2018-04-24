package padlockcloud

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/csrf"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"testing"
	"time"
)

const (
	testEmail = "martin@padlock.io"
	testData  = "Hello World!"
)

func init() {
	authMaxAge = func(authType string) time.Duration {
		switch authType {
		case "web":
			return time.Millisecond * 10
		default:
			return time.Duration(0)
		}
	}
}

type serverTestContext struct {
	server            *Server
	client            *http.Client
	storage           *MemoryStorage
	sender            *RecordSender
	host              string
	authToken         *AuthToken
	capturedAuthToken *AuthToken
	device            *Device
}

func (ctx *serverTestContext) resetStorage() {
	ctx.storage.Open()
}

func (ctx *serverTestContext) resetCookies() {
	jar, _ := cookiejar.New(nil)
	ctx.client.Jar = jar
}

func (ctx *serverTestContext) resetAll() {
	ctx.resetCookies()
	ctx.resetStorage()
	ctx.sender.Reset()
	ctx.authToken = nil
	ctx.capturedAuthToken = nil
}

func (ctx *serverTestContext) followRedirects(follow bool) {
	if follow {
		ctx.client.CheckRedirect = nil
	} else {
		ctx.client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
}

// Helper function for creating (optionally authenticated) requests
func (ctx *serverTestContext) request(method string, url string, body string, version int) (*http.Response, error) {
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

	if ctx.authToken != nil && ctx.authToken.Type == "api" {
		req.Header.Add("Authorization", ctx.authToken.String())
	}

	if ctx.device != nil {
		req.Header.Add("X-Device-Platform", ctx.device.Platform)
		req.Header.Add("X-Device-Model", ctx.device.Model)
		req.Header.Add("X-Device-Hostname", ctx.device.HostName)
		req.Header.Add("X-Device-App-Version", ctx.device.AppVersion)
		req.Header.Add("X-Device-OS-Version", ctx.device.OSVersion)
		req.Header.Add("X-Device-UUID", ctx.device.UUID)
		req.Header.Add("X-Device-Manufacturer", ctx.device.Manufacturer)
	}

	return ctx.client.Do(req)
}

func (ctx *serverTestContext) loginApi(email string) (*http.Response, error) {
	var res *http.Response
	var err error

	if res, err = ctx.request("POST", ctx.host+"/auth/", url.Values{
		"email": {email},
		"type":  {"api"},
	}.Encode(), ApiVersion); err != nil {
		return nil, err
	}

	responseBody, err := validateResponse(res, http.StatusAccepted, "")
	if err != nil {
		return res, err
	}

	ctx.authToken = &AuthToken{}
	// Response status code should be "ACCEPTED", response body should be the json-encoded auth token
	if err := json.Unmarshal(responseBody, ctx.authToken); err != nil {
		return res, fmt.Errorf("Failed to parse api key from response: %s", responseBody)
	}

	ctx.authToken.Type = "api"

	link, err := ctx.extractActivationLink()
	if err != nil {
		return res, err
	}

	return ctx.request("GET", link, "", 0)
}

func (ctx *serverTestContext) loginWeb(email string, redirect string) (*http.Response, error) {
	var res *http.Response
	var err error

	if res, err = ctx.request("POST", ctx.host+"/auth/", url.Values{
		"email":    {email},
		"type":     {"web"},
		"redirect": {redirect},
	}.Encode(), ApiVersion); err != nil {
		return nil, err
	}

	if _, err := validateResponse(res, http.StatusAccepted, ""); err != nil {
		return res, err
	}

	link, err := ctx.extractActivationLink()
	if err != nil {
		return res, err
	}

	// 'visit' activation link
	if res, err = ctx.request("GET", link, "", 0); err != nil {
		return res, err
	}

	u, _ := url.Parse(ctx.host)
	var authCookie *http.Cookie
	for _, c := range ctx.client.Jar.Cookies(u) {
		if c.Name == "auth" {
			authCookie = c
			break
		}
	}

	if authCookie == nil {
		return res, errors.New("Expected cookie of name 'auth' to be set")
	}

	if ctx.authToken, err = AuthTokenFromString(authCookie.Value); err != nil {
		return res, fmt.Errorf("Failed to parse auth token from cookie. Error: %v", err)
	}

	ctx.authToken.Type = "web"

	return res, nil
}

func (ctx serverTestContext) getCsrfToken() string {
	res, _ := ctx.request("GET", ctx.host+"/csrftest/", "", 0)
	csrfToken, _ := validateResponse(res, http.StatusOK, "")
	return string(csrfToken)
}

func (ctx *serverTestContext) extractActivationLink() (string, error) {
	// Activation message should be sent to the correct email
	if ctx.sender.Recipient != testEmail {
		return "", fmt.Errorf("Expected activation message to be sent to %s, instead got %s", testEmail, ctx.sender.Recipient)
	}

	// Activation message should contain a valid activation link
	linkPattern := fmt.Sprintf("%s/a/\\?t=%s", ctx.host, tokenPattern)
	msgPattern := fmt.Sprintf("%s, %s", testEmail, linkPattern)
	match, _ := regexp.MatchString(msgPattern, ctx.sender.Message)
	if !match {
		return "", fmt.Errorf("Expected activation message to match \"%s\", got \"%s\"", msgPattern, ctx.sender.Message)
	}
	link := regexp.MustCompile(linkPattern).FindString(ctx.sender.Message)

	return link, nil
}

func newServerTestContextWithConfig(serverConfig *ServerConfig) *serverTestContext {

	var context *serverTestContext

	var captureAuthToken = HandlerFunc(func(w http.ResponseWriter, r *http.Request, auth *AuthToken) error {
		context.capturedAuthToken = auth
		return nil
	})

	storage := &MemoryStorage{}
	sender := &RecordSender{}
	templates := &Templates{
		template.New(""),
		template.New(""),
		template.Must(template.New("").Parse("{{ .token.Email }}, {{ .activation_link }}")),
		template.Must(template.New("").Parse("")),
		template.Must(template.New("").Parse("<html>{{ .message }}</html>")),
		template.Must(template.New("").Parse("login,{{ .email }},{{ .submitted }}")),
		template.Must(template.New("").Parse("dashboard")),
	}

	logger := &Log{Config: &LogConfig{}}
	server := NewServer(logger, storage, sender, serverConfig)
	server.Templates = templates
	server.Init()
	logger.Info.SetOutput(ioutil.Discard)
	logger.Error.SetOutput(ioutil.Discard)

	server.Endpoints["/csrftest/"] = &Endpoint{
		AuthType: "web",
		Handlers: map[string]Handler{
			"GET": HandlerFunc(func(w http.ResponseWriter, r *http.Request, auth *AuthToken) error {
				w.Write([]byte(csrf.Token(r)))
				return nil
			}),
			"POST": HandlerFunc(func(w http.ResponseWriter, r *http.Request, auth *AuthToken) error {
				return nil
			}),
		},
	}

	server.Endpoints["/authtestnoauth/"] = &Endpoint{
		AuthType: "",
		Handlers: map[string]Handler{
			"GET": captureAuthToken,
		},
	}

	server.Endpoints["/authtestapi/"] = &Endpoint{
		AuthType: "api",
		Handlers: map[string]Handler{
			"GET": captureAuthToken,
		},
	}

	server.Endpoints["/authtestweb/"] = &Endpoint{
		AuthType: "web",
		Handlers: map[string]Handler{
			"GET": captureAuthToken,
		},
	}

	server.Endpoints["/panic/"] = &Endpoint{
		Handlers: map[string]Handler{
			"GET": HandlerFunc(func(w http.ResponseWriter, r *http.Request, a *AuthToken) error {
				panic("Everyone panic!!!")
			}),
			"POST": HandlerFunc(func(w http.ResponseWriter, r *http.Request, a *AuthToken) error {
				panic(errors.New("Everyone panic!!!"))
			}),
		},
	}

	server.InitHandler()

	server.emailRateLimiter = nil

	testServer := httptest.NewServer(server.Handler)

	host := testServer.URL

	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar: jar,
	}

	context = &serverTestContext{
		server:  server,
		client:  client,
		storage: storage,
		sender:  sender,
		host:    host,
	}

	return context
}

func newServerTestContext() *serverTestContext {
	return newServerTestContextWithConfig(&ServerConfig{})
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

func TestAuthentication(t *testing.T) {
	var res *http.Response
	var err error

	ctx := newServerTestContext()
	ctx.followRedirects(false)

	t.Run("account not found", func(t *testing.T) {
		ctx.resetAll()

		// Trying to get an api key for a non-existing account using the PUT method should result in a 404
		if res, err = ctx.request("PUT", ctx.host+"/auth/", url.Values{
			"email": {"hello@world.com"},
		}.Encode(), ApiVersion); err != nil {
			t.Fatal(err)
		}
		// No account with this email exists yet and we have not specified 'create=true' in our request
		testError(t, res, &AccountNotFound{})
	})

	t.Run("unauthenticated", func(t *testing.T) {
		ctx.resetAll()

		// Request should go through for route not requiring authentication
		if res, err = ctx.request("GET", ctx.host+"/authtestnoauth/", "", 0); err != nil {
			t.Fatal(err)
		}
		testResponse(t, res, http.StatusOK, "")

		if ctx.capturedAuthToken != nil {
			t.Error("Auth token to be passed to handler even though none was expected")
		}

		// Not logged in so we should be redirected to the login page
		if res, err = ctx.request("GET", ctx.host+"/authtestweb/", "", 0); err != nil {
			t.Fatal(err)
		}
		testResponse(t, res, http.StatusFound, "")
		if res.Header.Get("Location") != "/login/" {
			t.Error("Expected to be redirected to login page")
		}

		// Not authenticated so we should get a 401
		if res, err = ctx.request("GET", ctx.host+"/authtestapi/", "", 0); err != nil {
			t.Fatal(err)
		}
		testError(t, res, &InvalidAuthToken{})
	})

	t.Run("type=api", func(t *testing.T) {
		ctx.resetAll()

		if _, err = ctx.loginApi(testEmail); err != nil {
			t.Fatal(err)
		}

		// Route without authentication should always go through
		if res, err = ctx.request("GET", ctx.host+"/authtestnoauth/", "", 0); err != nil {
			t.Fatal(err)
		}
		testResponse(t, res, http.StatusOK, "")

		// Even though no authentication is required for this route, an auth token should still be
		// passed to the handler
		at := ctx.capturedAuthToken
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
		if res, err = ctx.request("GET", ctx.host+"/authtestweb/", "", 0); err != nil {
			t.Fatal(err)
		}
		testError(t, res, &InvalidAuthToken{})

		// We're authenticated so the request should go through
		if res, err = ctx.request("GET", ctx.host+"/authtestapi/", "", 0); err != nil {
			t.Fatal(err)
		}
		testResponse(t, res, http.StatusOK, "")
	})

	t.Run("type=web", func(t *testing.T) {
		ctx.resetAll()

		if res, err = ctx.loginWeb(testEmail, ""); err != nil {
			t.Fatal(err)
		}

		// By default user should be redirected to dasboard after login
		testResponse(t, res, http.StatusFound, "")
		if l := res.Header.Get("Location"); l != "/dashboard/" {
			t.Errorf("Expected redirect to %s, got %s", "/dashboard/", res.Header.Get("Location"))
		}

		// Test route without authentication
		if res, err = ctx.request("GET", ctx.host+"/authtestnoauth/", "", 0); err != nil {
			t.Fatal(err)
		}
		testResponse(t, res, http.StatusOK, "")

		at := ctx.capturedAuthToken
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
		if res, err = ctx.request("GET", ctx.host+"/authtestapi/", "", 0); err != nil {
			t.Fatal(err)
		}
		testError(t, res, &InvalidAuthToken{})

		// We're logged in so the request should go through
		if res, err = ctx.request("GET", ctx.host+"/authtestweb/", "", 0); err != nil {
			t.Fatal(err)
		}
		testResponse(t, res, http.StatusOK, "")

		// Make sure auth token expires
		time.Sleep(authMaxAge("web"))
		if res, err = ctx.request("GET", ctx.host+"/authtestweb/", "", 0); err != nil {
			t.Fatal(err)
		}
		testResponse(t, res, http.StatusFound, "")
		if res.Header.Get("Location") != "/login/" {
			t.Error("Expected to be redirected to login page")
		}

		// Redirect to other supported endpoints is also allowed
		if res, err = ctx.loginWeb(testEmail, "/authtestweb/?query"); err != nil {
			t.Fatal(err)
		}
		testResponse(t, res, http.StatusFound, "")
		if l := res.Header.Get("Location"); l != "/authtestweb/?query" {
			t.Errorf("Expected redirect to %s, got %s", "/authtestweb/?query", res.Header.Get("Location"))
		}

		// Using an external url or any unsupported endpoint should be treated as a bad request
		res, err = ctx.loginWeb(testEmail, "http://attacker.com")
		testError(t, res, &BadRequest{"invalid redirect path"})

		res, err = ctx.loginWeb(testEmail, "/notsupported/")
		testError(t, res, &BadRequest{"invalid redirect path"})
	})

	t.Run("invalid activation token", func(t *testing.T) {
		// An invalid activation token should result in a bad request response
		res, _ = ctx.request("GET", ctx.host+"/a/?t=asdf", "", ApiVersion)
		testError(t, res, &BadRequest{"invalid activation token"})
	})

	t.Run("whitelist", func(t *testing.T) {
		dir, err := ioutil.TempDir("", "")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(dir)

		tmpFile := filepath.Join(dir, "tmpFile")
		d1 := []byte(testEmail + "\n")
		if err := ioutil.WriteFile(tmpFile, d1, 0644); err != nil {
			t.Fatalf("Error writing to whitelist file: %s", err.Error())
		}

		//setup config with whitelist
		whitelistCtx := newServerTestContextWithConfig(&ServerConfig{WhitelistPath: tmpFile})
		whitelistCtx.followRedirects(false)

		randomEmail := "random@random.com"
		res, _ = whitelistCtx.loginApi(randomEmail)
		testError(t, res, &BadRequest{"invalid email address"})

		if _, err = whitelistCtx.loginApi(testEmail); err != nil {
			t.Errorf("Should have been able to login because %s is on whitelist: %s", whitelistTestEmail, err.Error())
		}
	})
}

func TestCsrfProtection(t *testing.T) {
	var res *http.Response
	var err error

	ctx := newServerTestContext()
	ctx.followRedirects(true)

	if res, err = ctx.loginWeb(testEmail, "/csrftest/"); err != nil {
		t.Fatal(err)
	}

	csrfToken, err := validateResponse(res, http.StatusOK, "")
	if err != nil {
		t.Fatal(err)
	}

	if res, err = ctx.request("POST", ctx.host+"/csrftest/", url.Values{
		"gorilla.csrf.Token": {"asdf"},
	}.Encode(), ApiVersion); err != nil {
		t.Fatal(err)
	}
	testError(t, res, &InvalidCsrfToken{})

	if res, err = ctx.request("POST", ctx.host+"/csrftest/", url.Values{
		"gorilla.csrf.Token": {string(csrfToken)},
	}.Encode(), ApiVersion); err != nil {
		t.Fatal(err)
	}
	testResponse(t, res, http.StatusOK, "")
}

func TestStore(t *testing.T) {
	var res *http.Response
	var err error

	ctx := newServerTestContext()
	ctx.followRedirects(true)

	t.Run("read(unauthenticated)", func(t *testing.T) {
		// Get data request authenticated with obtained api key should return with status code 200 - OK and
		// empty response body (since we haven't written any data yet)
		if res, err = ctx.request("GET", ctx.host+"/store/", "", ApiVersion); err != nil {
			t.Fatal(err)
		}
		testError(t, res, &InvalidAuthToken{})
	})

	if _, err := ctx.loginApi(testEmail); err != nil {
		t.Fatal(err)
	}

	t.Run("read(empty)", func(t *testing.T) {
		// Get data request authenticated with obtained api key should return with status code 200 - OK and
		// empty response body (since we haven't written any data yet)
		if res, err = ctx.request("GET", ctx.host+"/store/", "", ApiVersion); err != nil {
			t.Fatal(err)
		}
		testResponse(t, res, http.StatusOK, "^$")
	})

	t.Run("write", func(t *testing.T) {
		// Put request should return with status code 204 - NO CONTENT
		if res, err = ctx.request("PUT", ctx.host+"/store/", testData, ApiVersion); err != nil {
			t.Fatal(err)
		}
		testResponse(t, res, http.StatusNoContent, "")
	})

	t.Run("read(non-empty)", func(t *testing.T) {
		// Now get data request should return the data previously saved through PUT
		if res, err = ctx.request("GET", ctx.host+"/store/", "", ApiVersion); err != nil {
			t.Fatal(err)
		}
		testResponse(t, res, http.StatusOK, fmt.Sprintf("^%s$", testData))
	})

	t.Run("reset data", func(t *testing.T) {
		ctx.loginWeb(testEmail, "")

		// Revoke auth token by id
		if res, err = ctx.request("POST", ctx.host+"/deletestore/", url.Values{
			"gorilla.csrf.Token": {ctx.getCsrfToken()},
		}.Encode(), 0); err != nil {
			t.Fatal(err)
		}
		testResponse(t, res, http.StatusOK, "")

		ctx.loginApi(testEmail)
		// Store should be empty
		if res, err = ctx.request("GET", ctx.host+"/store/", "", ApiVersion); err != nil {
			t.Fatal(err)
		}
		testResponse(t, res, http.StatusOK, "^$")
	})
}

func TestDashboard(t *testing.T) {
	ctx := newServerTestContext()
	ctx.followRedirects(true)

	// If not logged in, should redirect to login page
	res, _ := ctx.request("GET", ctx.host+"/dashboard/", "", 0)
	testResponse(t, res, http.StatusOK, "^login,,$")

	if _, err := ctx.loginWeb(testEmail, ""); err != nil {
		t.Fatal(err)
	}

	// We should be logged in now, so dashboard should render
	res, _ = ctx.request("GET", ctx.host+"/dashboard/", "", 0)
	testResponse(t, res, http.StatusOK, "^dashboard$")
}

func TestLogout(t *testing.T) {
	var res *http.Response
	var err error

	ctx := newServerTestContext()

	if _, err = ctx.loginWeb(testEmail, ""); err != nil {
		t.Fatal(err)
	}

	// Log out
	if res, err = ctx.request("GET", ctx.host+"/logout/", "", 0); err != nil {
		t.Fatal(err)
	}
	testResponse(t, res, http.StatusOK, "")

	// If not logged in, should redirect to login page
	if res, err = ctx.request("GET", ctx.host+"/dashboard/", "", 0); err != nil {
		t.Fatal(err)
	}
	testResponse(t, res, http.StatusOK, "^login,,$")
}

func TestRevokeAuthToken(t *testing.T) {
	var res *http.Response
	var err error

	ctx := newServerTestContext()
	ctx.followRedirects(true)

	t.Run("unautenticated", func(t *testing.T) {
		// If not logged in, should redirect to login page
		if res, err = ctx.request("POST", ctx.host+"/revoke/", "", ApiVersion); err != nil {
			t.Fatal(err)
		}
		testResponse(t, res, http.StatusOK, "login,,")
	})

	// Login in
	if res, err = ctx.loginWeb(testEmail, ""); err != nil {
		t.Fatal(err)
	}

	t.Run("revoke by token", func(t *testing.T) {
		// Create and activate new auth token
		if res, err = ctx.loginApi(testEmail); err != nil {
			t.Fatal(err)
		}
		at := ctx.authToken
		ctx.authToken = nil

		// Revoke auth token by token
		if res, err = ctx.request("POST", ctx.host+"/revoke/", url.Values{
			"gorilla.csrf.Token": {ctx.getCsrfToken()},
			"token":              {at.Token},
		}.Encode(), 0); err != nil {
			t.Fatal(err)
		}
		testResponse(t, res, http.StatusOK, "")

		ctx.authToken = at
		if res, err = ctx.request("GET", ctx.host+"/store/", "", ApiVersion); err != nil {
			t.Fatal(err)
		}
		testError(t, res, &ExpiredAuthToken{})
	})

	t.Run("revoke by id", func(t *testing.T) {
		// Create and activate new auth token
		if res, err = ctx.loginApi(testEmail); err != nil {
			t.Fatal(err)
		}
		at := ctx.authToken
		ctx.authToken = nil

		// Get id
		acc := &Account{Email: at.Email}
		ctx.storage.Get(acc)
		at.Validate(acc)

		// Revoke auth token by id
		if res, err = ctx.request("POST", ctx.host+"/revoke/", url.Values{
			"gorilla.csrf.Token": {ctx.getCsrfToken()},
			"id":                 {at.Id},
		}.Encode(), 0); err != nil {
			t.Fatal(err)
		}
		testResponse(t, res, http.StatusOK, "")

		ctx.authToken = at
		if res, err = ctx.request("GET", ctx.host+"/store/", "", ApiVersion); err != nil {
			t.Fatal(err)
		}
		testError(t, res, &ExpiredAuthToken{})
	})
}

func TestMethodNotAllowed(t *testing.T) {
	ctx := newServerTestContext()
	// Requests with unsupported HTTP methods should return with 405 - method not allowed
	if res, err := ctx.request("GET", ctx.host+"/auth/", "", ApiVersion); err != nil {
		t.Fatal(err)
	} else {
		testError(t, res, &MethodNotAllowed{})
	}
}

func TestUnsupportedEndpoint(t *testing.T) {
	ctx := newServerTestContext()
	// Requests to unsupported paths should return with 404 - not found
	if res, err := ctx.request("GET", ctx.host+"/invalidpath", "", ApiVersion); err != nil {
		t.Fatal(err)
	} else {
		testError(t, res, &UnsupportedEndpoint{})
	}
}

func TestOutdatedVersion(t *testing.T) {
	ctx := newServerTestContext()

	// The root path is a special case in that the only way to figure out if the client is using
	// and older api version is if the Authorization header is using the 'ApiKey' authentication scheme
	token, _ := token()
	req, _ := http.NewRequest("GET", ctx.host+"/", nil)
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("ApiKey %s:%s", testEmail, token))
	res, _ := ctx.client.Do(req)
	testError(t, res, &UnsupportedApiVersion{})
	if ctx.sender.Recipient != testEmail {
		t.Errorf("Expected outdated message to be sent to %s, instead got %s", testEmail, ctx.sender.Recipient)
	}

	ctx.resetAll()

	// When doing an auth request, the email form field should be used for sending the notification since
	// the user is not authenticated
	res, _ = ctx.request("POST", ctx.host+"/auth/", url.Values{
		"email": {testEmail},
	}.Encode(), 0)
	testError(t, res, &UnsupportedApiVersion{0, ApiVersion})
	if ctx.sender.Recipient != testEmail {
		t.Errorf("Expected outdated message to be sent to %s, instead got %s", testEmail, ctx.sender.Recipient)
	}

	ctx.resetAll()

	ctx.loginApi(testEmail)

	// When doing an auth request, the email form field should be used for sending the notification since
	// the user is not authenticated
	res, _ = ctx.request("GET", ctx.host+"/store/", url.Values{
		"email": {testEmail},
	}.Encode(), 0)
	testError(t, res, &UnsupportedApiVersion{0, ApiVersion})
	if ctx.sender.Recipient != testEmail {
		t.Errorf("Expected outdated message to be sent to %s, instead got %s", testEmail, ctx.sender.Recipient)
	}
}

func TestPanicRecovery(t *testing.T) {
	ctx := newServerTestContext()
	res, _ := ctx.request("GET", ctx.host+"/panic/", "", 0)
	testError(t, res, &ServerError{})
	res, _ = ctx.request("POST", ctx.host+"/panic/", "", 0)
	testError(t, res, &ServerError{})
}

func TestErrorFormat(t *testing.T) {
	ctx := newServerTestContext()

	e := &UnsupportedEndpoint{}
	testErr := func(format string, expected []byte) {
		req, _ := http.NewRequest("GET", fmt.Sprintf("%s/invalidpath/?v=%d", ctx.host, ApiVersion), nil)
		if format != "" {
			req.Header.Add("Accept", format)
		}
		res, _ := ctx.client.Do(req)
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

func TestEmailRateLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	initRL := func(ctx *serverTestContext) {
		rl, _ := NewEmailRateLimiter(RateQuota{PerSec(1), 1}, RateQuota{PerSec(1), 1})
		ctx.server.emailRateLimiter = rl
	}

	request := func(ctx *serverTestContext, ip string, email string) (*http.Response, error) {
		req, err := http.NewRequest("POST", ctx.host+"/auth/", bytes.NewBuffer([]byte(url.Values{
			"email": {email},
		}.Encode())))
		if err != nil {
			return nil, err
		}

		req.RemoteAddr = ip
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Add("Accept", "application/vnd.padlock;version=1")

		w := httptest.NewRecorder()

		ctx.server.Handler.ServeHTTP(w, req)

		res := w.Result()
		res.Request = req

		return res, nil
	}

	t.Run("const_ip", func(t *testing.T) {
		var res *http.Response
		var err error

		t.Parallel()

		ctx := newServerTestContext()
		initRL(ctx)

		// One request per second with a burst of 1 additional request is allowed
		if res, err = request(ctx, "1.2.3.4", "email1"); err != nil {
			t.Fatal(err)
		}
		testResponse(t, res, http.StatusAccepted, "")

		if res, err = request(ctx, "1.2.3.4", "email2"); err != nil {
			t.Fatal(err)
		}
		testResponse(t, res, http.StatusAccepted, "")

		if res, err = request(ctx, "1.2.3.4", "email3"); err != nil {
			t.Fatal(err)
		}
		testError(t, res, &RateLimitExceeded{})

		// Requests with different ip should still go through
		if res, err = request(ctx, "1.2.3.5", "email4"); err != nil {
			t.Fatal(err)
		}
		testResponse(t, res, http.StatusAccepted, "")

		// After a second of wait, request should go through again
		time.Sleep(time.Second)
		if res, err = request(ctx, "1.2.3.4", "email5"); err != nil {
			t.Fatal(err)
		}
		testResponse(t, res, http.StatusAccepted, "")
	})

	t.Run("const_email", func(t *testing.T) {
		var res *http.Response
		var err error

		t.Parallel()

		ctx := newServerTestContext()
		initRL(ctx)

		// One request per second with a burst of 1 additional request is allowed
		if res, err = request(ctx, "1.2.3.4", "email1"); err != nil {
			t.Fatal(err)
		}
		testResponse(t, res, http.StatusAccepted, "")

		if res, err = request(ctx, "1.2.3.5", "email1"); err != nil {
			t.Fatal(err)
		}
		testResponse(t, res, http.StatusAccepted, "")

		if res, err = request(ctx, "1.2.3.6", "email1"); err != nil {
			t.Fatal(err)
		}
		testError(t, res, &RateLimitExceeded{})

		// Requests with different email should still go through
		if res, err = request(ctx, "1.2.3.7", "email2"); err != nil {
			t.Fatal(err)
		}
		testResponse(t, res, http.StatusAccepted, "")

		// After a second of wait, request should go through again
		time.Sleep(time.Second)
		if res, err = request(ctx, "1.2.3.8", "email1"); err != nil {
			t.Fatal(err)
		}
		testResponse(t, res, http.StatusAccepted, "")
	})

}

func TestDeviceData(t *testing.T) {
	ctx := newServerTestContext()

	ctx.device = &Device{
		Platform:     "darwin",
		UUID:         "uuid123",
		Manufacturer: "google",
		Model:        "Pixel",
		OSVersion:    "1.2.3",
		HostName:     "My Device",
		AppVersion:   "2.3.4",
	}

	ctx.loginApi(testEmail)

	acc := &Account{Email: testEmail}
	ctx.storage.Get(acc)

	var token *AuthToken
	if _, token = acc.findAuthToken(&AuthToken{Device: &Device{UUID: ctx.device.UUID}}); token == nil {
		t.Error("Should find auth token with the same device UUID")
	}

	if !reflect.DeepEqual(ctx.device, token.Device) {
		t.Errorf("Device data not saved correctly! Expected: %+v, got: %+v", ctx.device, token.Device)
	}

	ctx.device.OSVersion = "2.0.0"
	ctx.device.AppVersion = "3.0.0"
	ctx.device.HostName = "Some Hostname"

	ctx.request("GET", ctx.host+"/store/", "", 0)

	acc = &Account{Email: testEmail}
	ctx.storage.Get(acc)

	if _, token = acc.findAuthToken(&AuthToken{Device: &Device{UUID: ctx.device.UUID}}); token == nil {
		t.Error("Should find auth token with the same device UUID")
	}

	if !reflect.DeepEqual(ctx.device, token.Device) {
		t.Errorf("Device data not updated correctly! Expected: %+v, got: %+v", ctx.device, token.Device)
	}
}

func TestSecretInConfig(t *testing.T) {
	secret := bytes.Repeat([]byte("a"), 32)
	ctx := newServerTestContextWithConfig(&ServerConfig{Secret: base64.StdEncoding.EncodeToString(secret)})

	if !bytes.Equal(ctx.server.secret, secret) {
		t.Errorf("User-provided secret not set in server. Expected: %q, got: %q", secret, ctx.server.secret)
	}
}
