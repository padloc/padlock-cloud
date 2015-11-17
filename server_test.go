package main

import "testing"
import "os"
import "text/template"
import htmlTemplate "html/template"
import "net/http"
import "net/http/httptest"
import "net/url"
import "log"
import "io/ioutil"
import "regexp"
import "encoding/json"

type NullSender struct{}

func (s *NullSender) Send(string, string, string) error {
	return nil
}

var (
	app        *App
	server     *httptest.Server
	testEmail  = "martin@padlock.io"
	testDevice = "My Device"
)

func TestMain(m *testing.M) {
	log.Println("test main")
	app = &App{}
	app.Storage = &MemoryStorage{}
	app.Sender = &NullSender{}
	app.Templates = &Templates{
		template.Must(template.New("").Parse("{{ .device_name }}, {{ .email }}, {{ .activation_link }}")),
		template.Must(template.New("").Parse("{{ .email }}, {{ .delete_link }}")),
		htmlTemplate.Must(htmlTemplate.New("").Parse("{{ .device_name }}")),
		htmlTemplate.Must(htmlTemplate.New("").Parse("{{ .email }}")),
	}
	app.Init()
	app.Storage.Open()
	defer app.Storage.Close()

	server = httptest.NewServer(app)

	os.Exit(m.Run())
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

func TestFullCycle(t *testing.T) {
	res, _ := http.Get(server.URL)
	checkResponse(t, res, http.StatusUnauthorized, "\\s")

	res, err := http.PostForm(server.URL+"/auth", url.Values{
		"device_name": {testDevice},
		"email":       {testEmail},
	})
	if err != nil {
		log.Fatal(err)
	}

	body := checkResponse(t, res, http.StatusCreated, "")
	apiKey := &ApiKey{}

	err = json.Unmarshal(body, apiKey)
	if err != nil {
		t.Error("Failed to parse response body into ApiKey object", err)
	}

	if apiKey.Email != testEmail {
		t.Errorf("Expected email to match %s, is %s", testEmail, apiKey.Email)
	}

	if apiKey.DeviceName != testDevice {
		t.Errorf("Expected email to match %s, is %s", testDevice, apiKey.DeviceName)
	}

	match, err := regexp.MatchString(uuidPattern, apiKey.Key)
	if err != nil {
		log.Fatal(err)
	}
	if !match {
		t.Errorf("Expected %s to be a RFC4122-compliant uuid")
	}
}

//
// func TestAuthRequest(t *testing.T) {
//
// }
