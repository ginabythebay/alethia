package main

import (
	"crypto/tls"
	"log"
	"time"

	"github.com/mxk/go-imap/imap"
)

type Client struct {
	c          *imap.Client
	sentFolder string
}

func Dial(server string, insecureSkipVerify bool, user string, password string, sentFolder string) (c *Client, err error) {
	client, err := imap.Dial(server)
	if err != nil {
		return nil, err
	}

	log.Printf("Server says hello: %v, %v", client.Data[0].Info, client.Caps)

	// not sure why we are doing this, but it is in the example code
	client.Data = nil

	// Enable encryption, if supported by the server
	if client.Caps["STARTTLS"] {
		config := tls.Config{InsecureSkipVerify: insecureSkipVerify}
		if _, err := client.StartTLS(&config); err != nil {
			client.Logout(0)
			return nil, err
		}
	}

	// Authenticate
	if client.State() == imap.Login {
		log.Println("logging in")
		if _, err := client.Login(user, password); err != nil {
			client.Logout(0)
			return nil, err
		}
	}

	if _, err := client.Select(sentFolder, false); err != nil {
		client.Logout(0)
		return nil, err
	}
	logger.Print("\nMailbox status:\n", client.Mailbox)

	return &Client{client, sentFolder}, nil
}

func (c *Client) Logout(timeout time.Duration) {
	c.c.Logout(timeout)
}

func (c *Client) Save(b []byte) (err error) {
	literal := imap.NewLiteral(b)
	_, err = imap.Wait(c.c.Append(c.sentFolder, imap.NewFlagSet("\\Seen"), nil, literal))
	return
}
