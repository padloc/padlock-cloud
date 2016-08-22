package padlockcloud

import "fmt"
import "net/smtp"

// Sender is a interface that exposes the `Send` method for sending messages with a subject to a given
// receiver.
type Sender interface {
	Send(receiver string, subject string, message string) error
}

type EmailConfig struct {
	// User name used for authentication with the mail server
	User string
	// Mail server address
	Server string
	// Port on which to contact the mail server
	Port string
	// Password used for authentication with the mail server
	Password string
}

// EmailSender implements the `Sender` interface for emails
type EmailSender struct {
	EmailConfig
}

// Attempts to send an email to a given receiver. Through `smpt.SendMail`
func (sender *EmailSender) Send(rec string, subject string, body string) error {
	auth := smtp.PlainAuth(
		"",
		sender.User,
		sender.Password,
		sender.Server,
	)

	message := fmt.Sprintf("Subject: %s\r\nFrom: Padlock Cloud <%s>\r\n\r\n%s", subject, sender.User, body)
	return smtp.SendMail(
		sender.Server+":"+sender.Port,
		auth,
		sender.User,
		[]string{rec},
		[]byte(message),
	)
}

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
