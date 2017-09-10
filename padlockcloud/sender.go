package padlockcloud

import "fmt"
import "net/smtp"
import "strings"
import "github.com/aws/aws-sdk-go/aws"
import "github.com/aws/aws-sdk-go/aws/credentials"
import "github.com/aws/aws-sdk-go/aws/session"
import "github.com/aws/aws-sdk-go/service/ses"
import "log"

// Sender is a interface that exposes the `Send` method for sending messages with a subject to a given
// recipient.
type Sender interface {
	Send(recipient string, subject string, message string) error
}

type EmailConfig struct {
	// Email Provider SMTP or AWS SES
	Provider string `yaml:"email_provider"`
	// Set the AWS SES Region
	AWSRegion string `yaml:"aws_region"`
	// AWS Access Key ID
	AWSAccessKey string `yaml:"aws_access_key"`
	// AWS Secret Key
	AWSSecretKey string `yaml:"aws_secret_key"`
	// AWS FROM Address
	SESFrom string `yaml:"ses_from"`
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

	// Send Email using SMTP Protocol
	if strings.ToLower(sender.Config.Provider) == "smtp" {
		//fmt.Println("Sending Email Using SMTP.")
		log.Print("Using SMTP For Sending Email.")
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

	// Send Email Using AWS SES
	if strings.ToLower(sender.Config.Provider) == "ses" {
		//fmt.Println("Sending Email Using AWS SES.")
		log.Print("Using AWS SES For Sending Email.")

		var sess *session.Session
		var err error

		// Determine whether the AWS Access Keys are Statically Provided.
		if sender.Config.AWSAccessKey != "" {
			log.Print("Using Statically defined credentials is a bad practice. Please see AWS Documentation for more info. https://docs.aws.amazon.com/sdk-for-go/v1/developer-guide/configuring-sdk.html#id2")
			sess, err = session.NewSession(&aws.Config{
				Region:      aws.String(sender.Config.AWSRegion),
				Credentials: credentials.NewStaticCredentials(sender.Config.AWSAccessKey, sender.Config.AWSSecretKey, ""),
			})
		} else {
			sess, err = session.NewSession(&aws.Config{Region: aws.String(sender.Config.AWSRegion)})
		}

		if err != nil {
			log.Print(err)
		}
		svc := ses.New(sess)

		params := &ses.SendEmailInput{
			Destination: &ses.Destination{ // Required
				ToAddresses: []*string{
					aws.String(rec), // Required
				},
			},
			Message: &ses.Message{ // Required
				Body: &ses.Body{ // Required
					Html: &ses.Content{
						Data: aws.String(body), // Required
					},
					Text: &ses.Content{
						Data: aws.String(body), // Required
					},
				},
				Subject: &ses.Content{ // Required
					Data: aws.String("From Padlock Cloud"), // Required
				},
			},
			Source: aws.String(sender.Config.SESFrom), // Required
		}
		//fmt.Println(params)
		_, err = svc.SendEmail(params)

		if err != nil {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			fmt.Println(err.Error())
			return err
		}

		// Pretty-print the response data.
		//fmt.Println(resp)

	}
	return nil
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
