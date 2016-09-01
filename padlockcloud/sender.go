package padlockcloud

import "fmt"
import "net/smtp"

// Sender is a interface that exposes the `Send` method for sending messages with a subject to a given
// recipient.
type Sender interface {
	Send(recipient string, subject string, message string) error
}

type EmailConfig struct {
	// User name used for authentication with the mail server
	User string `yaml:"user"`
	// Mail server address
	Server string `yaml:"server"`
	// Port on which to contact the mail server
	Port string `yaml:"port"`
	// Password used for authentication with the mail server
	Password string `yaml:"password"`
}

// EmailSender implements the `Sender` interface for emails
type EmailSender struct {
	Config *EmailConfig
}

// Attempts to send an email to a given recipient. Through `smpt.SendMail`
func (sender *EmailSender) Send(rec string, subject string, body string) error {
	auth := smtp.PlainAuth(
		"",
		sender.Config.User,
		sender.Config.Password,
		sender.Config.Server,
	)

	message := fmt.Sprintf("Subject: %s\r\nFrom: Padlock Cloud <%s>\r\n\r\n%s", subject, sender.Config.User, body)
	return smtp.SendMail(
		sender.Config.Server+":"+sender.Config.Port,
		auth,
		sender.Config.User,
		[]string{rec},
		[]byte(message),
	)
}

// Mock implementation of the `Sender` interface. Simply records arguments passed to the `Send` method
type RecordSender struct {
	Recipient string
	Subject   string
	Message   string
}

func (s *RecordSender) Send(rec string, subj string, message string) error {
	s.Recipient = rec
	s.Subject = subj
	s.Message = message
	return nil
}

func (s *RecordSender) Reset() {
	s.Recipient = ""
	s.Subject = ""
	s.Message = ""
}
