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
	// Sender mail address for outgoing mails. If empty, `User` is used instead.
	From string `yaml:"from"`
}

// EmailSender implements the `Sender` interface for emails
type EmailSender struct {
	Config *EmailConfig
	// Function used to actually send the mail. Same signature as `smtp.SendMail`.
	SendFunc func(addr string, a smtp.Auth, from string, to []string, msg []byte) error
}

// NewEmailSender returns an EmailSender which sends mail using `smtp.SendMail`.
// Its configuration points to the given `EmailConfig`.
func NewEmailSender(c *EmailConfig) *EmailSender {
	return &EmailSender{
		Config:   c,
		SendFunc: smtp.SendMail,
	}
}

// Attempts to send an email to a given recipient.
func (sender *EmailSender) Send(rec string, subject string, body string) error {
	var auth smtp.Auth
	if sender.Config.User != "" {
		auth = smtp.PlainAuth(
			"",
			sender.Config.User,
			sender.Config.Password,
			sender.Config.Server,
		)
	}

	from := sender.Config.From
	if from == "" {
		from = sender.Config.User
	}

	message := fmt.Sprintf("Subject: %s\r\nFrom: %s\r\nTo: %s\r\n\r\n%s", subject, from, rec, body)
	return sender.SendFunc(
		sender.Config.Server+":"+sender.Config.Port,
		auth,
		from,
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
