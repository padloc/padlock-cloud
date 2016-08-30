package padlockcloud

import "testing"
import "strings"
import "os"
import "io/ioutil"
import "bytes"
import "path/filepath"
import "time"

func TestLogStdout(t *testing.T) {
	// Replace standard outputs with buffer for recording
	testout := new(bytes.Buffer)
	testerr := new(bytes.Buffer)
	prevout := stdout
	preverr := stderr
	stdout = testout
	stderr = testerr
	defer func() {
		stdout = prevout
		stderr = preverr
	}()

	l := NewLog(&LogConfig{}, nil)

	testStr := "hello world!"
	l.Info.Print(testStr)
	if !strings.HasSuffix(testout.String(), testStr+"\n") {
		t.Fatalf("Standard output should end with printed string '%s', got '%s'", testStr, testout.String())
	}

	l.Error.Print(testStr)
	if !strings.HasSuffix(testerr.String(), testStr+"\n") {
		t.Fatalf("Error output should end with printed string '%s', got '%s'", testStr, testerr.String())
	}
}

func TestLogFile(t *testing.T) {
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	logfile := filepath.Join(dir, "LOG.txt")
	errfile := filepath.Join(dir, "ERR.txt")

	l := NewLog(&LogConfig{
		LogFile: logfile,
		ErrFile: errfile,
	}, nil)

	testStr := "hello world!"

	l.Info.Print(testStr)
	data, err := ioutil.ReadFile(logfile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(data), testStr+"\n") {
		t.Fatalf("Log file should end with last printed string, got %s", data)
	}

	l.Error.Print(testStr)
	data, err = ioutil.ReadFile(errfile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(data), testStr+"\n") {
		t.Fatalf("Error file should end with last printed string, got %s", data)
	}
}

func TestLogNotifyErrors(t *testing.T) {
	// Disable standard error output so it doesn'ts show up during tests
	preverr := stderr
	stderr = ioutil.Discard
	defer func() {
		stderr = preverr
	}()

	recipient := "me"
	subject := "Padlock Cloud Error Notification"
	rc := &RecordSender{}
	l := NewLog(&LogConfig{
		NotifyErrors: recipient,
	}, rc)

	testStr := "hello world"
	l.Error.Print(testStr)

	// We need to wait a little since the send method is called in a goroutine
	time.Sleep(time.Millisecond)

	if rc.Recipient != recipient {
		t.Fatalf("Wrong recipient. Expected '%s', got '%s'", recipient, rc.Recipient)
	}

	if rc.Subject != subject {
		t.Fatalf("Wrong subject. Expected '%s', got '%s'", subject, rc.Subject)
	}

	if strings.HasSuffix(rc.Message, testStr) {
		t.Fatalf("Expected message to end in printed string '%s', got '%s'", testStr, rc.Message)
	}
}
