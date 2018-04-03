package padlockcloud

import (
	"bytes"
	"errors"
	"net/smtp"
	"reflect"
	"testing"
)

func TestEmailSenderSendCallsSendMailCorrectly(t *testing.T) {
	sender := NewEmailSender(&EmailConfig{
		User:     "test_user",
		Server:   "test_server",
		Port:     "test_port",
		Password: "unused_password",
		From:     "test_from",
	})
	sender.SendFunc = func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		if want := "test_server:test_port"; addr != want {
			t.Errorf("SendFunc addr: got %q; want %q", addr, want)
		}
		// smtp.Auth is tested in a separate test.
		if want := "test_from"; from != want {
			t.Errorf("SendFunc from: got %q; want %q", from, want)
		}
		if want := []string{"test_recipient"}; !reflect.DeepEqual(to, want) {
			t.Errorf("SendFunc to: got %q; want %q", to, want)
		}
		if want := []byte("test_subject"); !bytes.Contains(msg, want) {
			t.Errorf("SendFunc msg: got %q; want contains %q", msg, want)
		}
		if want := []byte("test_body"); !bytes.Contains(msg, want) {
			t.Errorf("SendFunc msg: got %q; want contains %q", msg, want)
		}
		return nil
	}
	if err := sender.Send("test_recipient", "test_subject", "test_body"); err != nil {
		t.Errorf("Send() == %v; want nil", err)
	}
}

func TestEmailSenderSendFallsBackToUserWhenNoFrom(t *testing.T) {
	sender := NewEmailSender(&EmailConfig{
		User:     "test_user",
		Server:   "unused_server",
		Port:     "unused_port",
		Password: "unused_password",
		// No `From`: fall back to `User`.
	})
	sender.SendFunc = func(_ string, _ smtp.Auth, from string, _ []string, _ []byte) error {
		if want := "test_user"; from != want {
			t.Errorf("SendFunc from: got %q; want %q", from, want)
		}
		return nil
	}
	if err := sender.Send("test_recipient", "test_subject", "test_body"); err != nil {
		t.Errorf("Send() == %v; want nil", err)
	}

}

// This is tested separately as it relies on implementation details of smtp.PlainAuth.
// One way to make this test less brittle is to inject the auth dependency in EmailSender.
func TestEmailSenderSendBuildsPlainAuth(t *testing.T) {
	sender := NewEmailSender(&EmailConfig{
		User:     "test_user",
		Server:   "test_server",
		Port:     "unused_port",
		Password: "test_password",
	})
	sender.SendFunc = func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		_, toServer, err := a.Start(&smtp.ServerInfo{
			TLS:  true,
			Name: "test_server",
		})
		if err != nil {
			t.Fatalf("SendFunc auth: call to Start(): got %v; want nil", err)
		}
		if want := []byte("test_user"); !bytes.Contains(toServer, want) {
			t.Errorf("SendFunc auth: PlainAuth toServer: got %q; want contains %q", toServer, want)
		}
		if want := []byte("test_password"); !bytes.Contains(toServer, want) {
			t.Errorf("SendFunc auth: PlainAuth toServer: got %q; want contains %q", toServer, want)
		}
		return nil
	}
	if err := sender.Send("unused_recipient", "unused_subject", "unused_body"); err != nil {
		t.Errorf("Send() == %v; want nil", err)
	}
}

func TestEmailSenderNoUserNoAuth(t *testing.T) {
	sender := NewEmailSender(&EmailConfig{
		// No `User`: disable authentication.
		Server:   "unused_server",
		Port:     "unused_port",
		Password: "unused_password",
	})
	sender.SendFunc = func(_ string, a smtp.Auth, _ string, _ []string, _ []byte) error {
		if a != nil {
			t.Errorf("SendFunc auth: got %q; want nil", a)
		}
		return nil
	}
	if err := sender.Send("unused_recipient", "unused_subject", "unused_body"); err != nil {
		t.Errorf("Send() == %v; want nil", err)
	}
}

func TestEmailSenderSendFailsWhenSendMailFails(t *testing.T) {
	sender := NewEmailSender(&EmailConfig{
		User:     "user",
		Server:   "server",
		Port:     "42",
		Password: "password",
		From:     "unused_from",
	})
	want := errors.New("jabberwock")
	sender.SendFunc = func(string, smtp.Auth, string, []string, []byte) error {
		return want
	}
	if err := sender.Send("recipient", "subject", "body"); err != want {
		t.Errorf("sender.Send(): got %v; want %v", err, want)
	}
}
