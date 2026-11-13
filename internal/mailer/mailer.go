package mailer

import (
	"errors"
	"log"
	"net/smtp"
	"strings"

	"aarvix-nexus/internal/config"
)

type Mailer interface {
	Send(to, subject, body string) error
}

// New returns an SMTP mailer when SMTP_HOST is set; otherwise a DEV mailer that
// prints the message to the server log. Never run the dev mailer in production:
// it exposes verification codes in logs.
func New(cfg config.Config) Mailer {
	if cfg.SMTPHost == "" {
		log.Println("mailer: SMTP_HOST not set, verification codes will be LOGGED (dev mode)")
		return logMailer{}
	}
	return smtpMailer{cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPass, cfg.SMTPFrom}
}

type logMailer struct{}

func (logMailer) Send(to, subject, body string) error {
	log.Printf("[DEV MAIL] to=%s subject=%q body=%q", to, subject, body)
	return nil
}

type smtpMailer struct{ host, port, user, pass, from string }

func (m smtpMailer) Send(to, subject, body string) error {
	if strings.ContainsAny(to+subject, "\r\n") {
		return errors.New("invalid header characters")
	}
	msg := "From: " + m.from + "\r\nTo: " + to + "\r\nSubject: " + subject + "\r\n\r\n" + body
	return smtp.SendMail(m.host+":"+m.port, smtp.PlainAuth("", m.user, m.pass, m.host), m.from, []string{to}, []byte(msg))
}
