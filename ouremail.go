package main

import (
	"github.com/jordan-wright/email"
	"net/smtp"
	"net/textproto"
	"strings"
)

type Sender struct {
	smtpHost     string
	smtpPort     string
	smtpUser     string
	smtpPassword string
	hostPort     string
}

func NewSender(smtpHost string, smtpPort string, smtpUser string, smtpPassword string) (res *Sender) {
	tokens := []string{smtpHost, smtpPort}
	return &Sender{smtpHost, smtpPort, smtpUser, smtpPassword, strings.Join(tokens, ":")}
}

type OurEmail struct {
	s *Sender
	d *email.Email
}

func (s *Sender) NewEmail(headers textproto.MIMEHeader, text string) *OurEmail {
	e := email.NewEmail()
	if s, ok := headers["To"]; ok {
		e.To = s
	}
	if s, ok := headers["Cc"]; ok {
		e.Cc = s
	}
	if s, ok := headers["Bcc"]; ok {
		e.Bcc = s
	}
	if s := headers.Get("From"); len(s) != 0 {
		e.From = s
	}
	e.Headers = headers
	e.Text = []byte(text)

	return &OurEmail{s, e}
}

func (e *OurEmail) Send() error {
	auth := smtp.PlainAuth("", e.s.smtpUser, e.s.smtpPassword, e.s.smtpHost)
	return e.d.Send(e.s.hostPort, auth)
}

func (e *OurEmail) Bytes() ([]byte, error) {
	return e.d.Bytes()
}
