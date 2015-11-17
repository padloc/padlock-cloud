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
import "encoding/json"
import "bytes"

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

var (
	app        *App
	server     *httptest.Server
	storage    *MemoryStorage
	sender     *RecordSender
	testEmail  = "martin@padlock.io"
	testDevice = "My Device"
	testData   = "Hello World!"
)

func TestMain(m *testing.M) {
	storage = &MemoryStorage{}
	sender = &RecordSender{}
	templates := &Templates{
		template.Must(template.New("").Parse("{{ .device_name }}, {{ .email }}, {{ .activation_link }}")),
		template.Must(template.New("").Parse("{{ .email }}, {{ .delete_link }}")),
		htmlTemplate.Must(htmlTemplate.New("").Parse("{{ .device_name }}")),
		htmlTemplate.Must(htmlTemplate.New("").Parse("{{ .email }}")),
	}

	app = NewApp(storage, sender, templates, Config{RequireTLS: false})

	app.Storage.Open()
	defer app.Storage.Close()

	server = httptest.NewServer(app)

	os.Exit(m.Run())
}

func request(method string, path string, body string, apiKey *ApiKey) (*http.Response, error) {
	req, _ := http.NewRequest(method, server.URL+path, bytes.NewBuffer([]byte(body)))
	if apiKey != nil {
		req.Header.Add("Authorization", fmt.Sprintf("ApiKey %s:%s", apiKey.Email, apiKey.Key))
	}
	return http.DefaultClient.Do(req)
}

func checkResponse(t *testing.T, res *http.Response, code int, body string) []byte {
	if res.StatusCode != code {
		t.Errorf("Expected status code to be %d, is %d", code, res.StatusCode)
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
		t.Errorf("Expected response body to match \"%s\", is \"%s\"", body, resBody)
	}

	return resBody
}

func TestLifeCycle(t *testing.T) {
	res, _ := http.PostForm(server.URL+"/auth", url.Values{
		"device_name": {testDevice},
		"email":       {testEmail},
	})

	body := checkResponse(t, res, http.StatusCreated, "")
	apiKey := &ApiKey{}

	err := json.Unmarshal(body, apiKey)
	if err != nil {
		t.Error("Failed to parse response body into ApiKey object", err)
	}

	if apiKey.Email != testEmail {
		t.Errorf("Expected email to match %s, is %s", testEmail, apiKey.Email)
	}

	if apiKey.DeviceName != testDevice {
		t.Errorf("Expected email to match %s, is %s", testDevice, apiKey.DeviceName)
	}

	match, _ := regexp.MatchString(uuidPattern, apiKey.Key)
	if !match {
		t.Errorf("Expected %s to be a RFC4122-compliant uuid")
	}

	if sender.Receiver != testEmail {
		t.Errorf("Expected activation message to be sent to %s, instead got %s", testEmail, sender.Receiver)
	}

	linkPattern := server.URL + "/activate/" + uuidPattern
	msgPattern := fmt.Sprintf("%s, %s, %s", testDevice, testEmail, linkPattern)
	match, _ = regexp.MatchString(msgPattern, sender.Message)
	if !match {
		t.Errorf("Expected activation message to match \"%s\", got \"%s\"", msgPattern, sender.Message)
	}
	link := regexp.MustCompile(linkPattern).FindString(sender.Message)

	res, _ = http.Get(link)
	checkResponse(t, res, http.StatusOK, fmt.Sprintf("^%s$", testDevice))

	res, _ = request("GET", "", "", apiKey)
	checkResponse(t, res, http.StatusOK, "^$")

	res, _ = request("PUT", "", testData, apiKey)
	checkResponse(t, res, http.StatusNoContent, "")

	res, _ = request("GET", "", "", apiKey)
	checkResponse(t, res, http.StatusOK, fmt.Sprintf("^%s$", testData))

	res, _ = request("DELETE", "/"+testEmail, "", nil)
	checkResponse(t, res, http.StatusAccepted, "")

	linkPattern = server.URL + "/reset/" + uuidPattern
	msgPattern = fmt.Sprintf("%s, %s", testEmail, linkPattern)
	match, _ = regexp.MatchString(msgPattern, sender.Message)
	if !match {
		t.Errorf("Expected activation message to match \"%s\", got \"%s\"", msgPattern, sender.Message)
	}
	link = regexp.MustCompile(linkPattern).FindString(sender.Message)

	res, _ = http.Get(link)
	checkResponse(t, res, http.StatusOK, fmt.Sprintf("^%s$", testEmail))

	res, _ = request("GET", "", "", apiKey)
	checkResponse(t, res, http.StatusOK, "^$")
}

func TestErrorConditions(t *testing.T) {
	res, _ := request("GET", "", "", nil)
	checkResponse(t, res, http.StatusUnauthorized, "")

	res, _ = request("POST", "", "", nil)
	checkResponse(t, res, http.StatusMethodNotAllowed, "")

	res, _ = request("DELETE", "", "", nil)
	checkResponse(t, res, http.StatusMethodNotAllowed, "")

	res, _ = request("GET", "/auth", "", nil)
	checkResponse(t, res, http.StatusMethodNotAllowed, "")

	res, _ = request("GET", "/invalidpath", "", nil)
	checkResponse(t, res, http.StatusNotFound, "")

	app.RequireTLS = true
	res, _ = request("GET", "", "", nil)
	checkResponse(t, res, http.StatusForbidden, "")
}
