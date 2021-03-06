package smtp

import (
	"errors"
	"fmt"
	"github.com/viant/toolbox/url"
	"strings"
)

//SendRequest represents send request.
type SendRequest struct {
	Target *url.Resource `required:"true" description:"SMTP endpoint"`
	Mail   *MailMessage  `required:"true"`
	UDF    string        `description:"body UDF"`
}

//SendResponse represents send response.
type SendResponse struct {
	SendPayloadSize int
}

//Validate validates send request.
func (r *SendRequest) Validate() error {
	if r.Target == nil {
		return errors.New("target was nil")
	}
	if r.Target.Credentials == "" {
		return errors.New("credentials was empty")
	}
	if r.Mail == nil {
		return errors.New("mail was nil")
	}
	return r.Mail.Validate()
}

//MailMessage represent an email
type MailMessage struct {
	From        string `required:"true" description:"sender, has to match email from target.credentials"`
	To          []string
	Cc          []string
	Bcc         []string
	Subject     string
	Body        string
	ContentType string
}

//Validate checks if mail message is valid
func (m *MailMessage) Validate() error {
	if m.From == "" {
		return errors.New("mail.from was empty")
	}
	if m.Subject == "" {
		return errors.New("mail.subject was empty")
	}
	if len(m.To) == 0 {
		return errors.New("mail.to was empty")
	}
	return nil
}

func getHeader(key string, values ...string) string {
	if len(values) == 0 || values[0] == "" {
		return ""
	}
	return fmt.Sprintf("%v: %s\r\n", key, strings.Join(values, ";"))
}

//Receivers returns all receivers for this email message
func (m *MailMessage) Receivers() []string {
	var result = make([]string, 0)
	result = append(result, m.To...)
	result = append(result, m.Cc...)
	result = append(result, m.Bcc...)
	return result
}

//Payload returns mail payload.
func (m *MailMessage) Payload() []byte {
	var result = ""
	result += getHeader("From", m.From)
	result += getHeader("To", m.To...)
	result += getHeader("Cc", m.Cc...)
	result += getHeader("Bcc", m.Bcc...)
	result += getHeader("Subject", m.Subject)
	if m.ContentType != "" {
		result += fmt.Sprintf("MIME-version: 1.0;\nContent-Type: %v; charset=\"UTF-8\";\r\n", m.ContentType)
	}
	result += "\r\n" + m.Body
	return []byte(result)
}
