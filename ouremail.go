package main

import (
	"crypto/tls"
	"net/smtp"
	"net/textproto"
	"strings"

	"github.com/jordan-wright/email"
)

type Sender struct {
	insecureSkipVerify bool
	smtpHost           string
	smtpPort           string
	smtpUser           string
	smtpPassword       string
	hostPort           string
}

func NewSender(insecureSkipVerify bool, smtpHost string, smtpPort string, smtpUser string, smtpPassword string) (res *Sender) {
	tokens := []string{smtpHost, smtpPort}
	return &Sender{insecureSkipVerify, smtpHost, smtpPort, smtpUser, smtpPassword, strings.Join(tokens, ":")}
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
	fields, err := e.d.ExtractFields()
	if err != nil {
		return err
	}

	// This section is a lot like net/stmp.SendMail, but we allow for disabling TLS
	c, err := smtp.Dial(e.s.hostPort)
	if err != nil {
		return err
	}
	defer c.Close()
	if ok, _ := c.Extension("STARTTLS"); ok {
		config := tls.Config{InsecureSkipVerify: e.s.insecureSkipVerify}
		if err = c.StartTLS(&config); err != nil {
			return err
		}
	}
	if ok, _ := c.Extension("AUTH"); ok {
		auth := smtp.PlainAuth("", e.s.smtpUser, e.s.smtpPassword, e.s.smtpHost)
		if err = c.Auth(auth); err != nil {
			return err
		}
	}
	if err = c.Mail(fields.From); err != nil {
		return err
	}
	for _, addr := range fields.To {
		if err = c.Rcpt(addr); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	_, err = w.Write(fields.Msg)
	if err != nil {
		return err
	}
	err = w.Close()
	if err != nil {
		return err
	}
	return c.Quit()
}

func (e *OurEmail) Bytes() ([]byte, error) {
	return e.d.Bytes()
}
