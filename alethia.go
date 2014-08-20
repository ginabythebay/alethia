package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"flag"
	"github.com/jordan-wright/email"
	"github.com/mxk/go-imap/imap"
	"io"
	"log"
	"net/smtp"
	"net/textproto"
	"os"
	"strings"
	"text/template"
	"time"
)

const (
	DEFAULT_SMTP_PORT = "25"
)

var (
	logger         = log.New(os.Stderr, "", log.Lshortfile)
	namedValueFlag = make(namedValue, 10)
)

func readTemplate(templateFileName string) (headers *template.Template, body *template.Template, err error) {
	file, err := os.Open(templateFileName)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()

	// The header will be first, separated from the body by a blank line
	inBody := false
	headerLines := make([]string, 0, 10)
	bodyLines := make([]string, 0, 50)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		nextLine := scanner.Text()
		switch {
		case inBody:
			bodyLines = append(bodyLines, nextLine)
		case len(strings.TrimSpace(nextLine)) == 0:
			inBody = true
			// drop this line
		default:
			headerLines = append(headerLines, nextLine)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}

	headerTemplate, err := template.New("headers").Parse(strings.Join(headerLines, "\n"))
	if err != nil {
		return nil, nil, err
	}
	bodyTemplate, err := template.New("body").Parse(strings.Join(bodyLines, "\n"))
	if err != nil {
		return nil, nil, err
	}

	return headerTemplate, bodyTemplate, nil
}

func executeTemplate(t *template.Template, data interface{}) (text string, err error) {
	var b bytes.Buffer
	err = t.Execute(&b, data)
	switch {
	case err != nil:
		return "", err
	default:
		return b.String(), nil
	}
}

func parseSmtpServer(smtpServer string) (host string, port string) {
	tokens := strings.SplitN(smtpServer, ":", 2)
	switch {
	case len(tokens) == 2:
		return tokens[0], tokens[1]
	default:
		return smtpServer, DEFAULT_SMTP_PORT
	}
}

func combine(record map[string]string, nv namedValue) map[string]string {
	// Combines record and nv into a new map, which any entries from
	// nv overriding those in record.
	res := map[string]string{}
	for k, v := range record {
		res[k] = v
	}
	for k, v := range nv {
		res[k] = v
	}
	return res
}

func main() {
	smtpServer := flag.String("smtpServer", "", "name (or ip) of smtp server")
	smtpUser := flag.String("smtpUser", "", "user for smtp log in")
	smtpPassword := flag.String("smtpPassword", "", "password for smtp log in")

	imapServer := flag.String("imapServer", "", "name (or ip) of imap server")
	imapUser := flag.String("imapUser", "", "user for imap log in")
	imapPassword := flag.String("imapPassword", "", "password for imap log in")
	imapSent := flag.String("imapSent", "INBOX.Sent", "Folder on imap server for sent mail")
	templateFile := flag.String("templateFile", "", "Name of file containing template")
	tabularFileName := flag.String("tabularFile", "", "Name of tsv or csv file (one row per email)")
	insecureSkipVerify := flag.Bool("insecureSkipVerify", false, "If set, disables checking of hosts certificate chain and host name (needed with some dreamhost servers because they use a single certificate for all mail hosts")
	flag.Var(namedValueFlag, "nv", "key=value pair that will be used in template.  This flag may be specified multiple times")

	flag.Parse()

	smtpHost, smtpPort := parseSmtpServer(*smtpServer)

	headersTemplate, bodyTemplate, err := readTemplate(*templateFile)
	if err != nil {
		log.Fatal(err)
	}

	tabularFile, err := os.Open(*tabularFileName)
	if err != nil {
		log.Fatal(err)
	}
	defer tabularFile.Close()

	tabularReader, err := NewTabularReader(tabularFile)
	if err != nil {
		log.Fatal(err)
	}

	logger.Printf("imapServer %#v\n", *imapServer)

	c, err := imap.Dial(*imapServer)
	if err != nil {
		log.Fatal(err)
	}

	// Remember to log out and close the connection when finished
	defer c.Logout(30 * time.Second)

	// Print server greeting (first response in the unilateral server data queue)
	logger.Printf("Server says hello: %v, %v", c.Data[0].Info, c.Caps)
	c.Data = nil

	// Enable encryption, if supported by the server
	if c.Caps["STARTTLS"] {
		logger.Println("Starting TLS")
		Config := tls.Config{InsecureSkipVerify: *insecureSkipVerify}
		if _, err := c.StartTLS(&Config); err != nil {
			log.Fatal(err)
		}
	}

	// Authenticate
	if c.State() == imap.Login {
		logger.Println("logging in")
		if _, err := c.Login(*imapUser, *imapPassword); err != nil {
			log.Fatal(err)
		}
	}

	if _, err := c.Select(*imapSent, false); err != nil {
		log.Fatal(err)
	}
	logger.Print("\nMailbox status:\n", c.Mailbox)

Loop:
	for {
		record, err := tabularReader.Read()
		switch {
		case err == io.EOF:
			break Loop
		case err != nil:
			logger.Print(err)
			break Loop
		}
		templateValues := combine(record, namedValueFlag)

		headerText, err := executeTemplate(headersTemplate, templateValues)
		if err != nil {
			// maybe I need to keep track of whether there were any
			// errors and at the end if there were, decline to
			// actually send anything (unless a force flag was set).
			logger.Print(err)
			break Loop
		}
		rr := textproto.NewReader(bufio.NewReader(strings.NewReader(headerText)))
		mimeHeader, err := rr.ReadMIMEHeader()
		if err != nil && err != io.EOF {
			log.Fatal(err)
		}

		bodyText, err := executeTemplate(bodyTemplate, templateValues)
		if err != nil {
			// maybe I need to keep track of whether there were any
			// errors and at the end if there were, decline to
			// actually send anything (unless a force flag was set).
			logger.Print(err)
			break Loop
		}

		e := email.NewEmail()
		if s, ok := mimeHeader["To"]; ok {
			e.To = s
		}
		if s, ok := mimeHeader["Cc"]; ok {
			e.Cc = s
		}
		if s, ok := mimeHeader["Bcc"]; ok {
			e.Bcc = s
		}
		if s := mimeHeader.Get("From"); len(s) != 0 {
			e.From = s
		}
		e.Headers = mimeHeader
		e.Text = []byte(bodyText)
		emailBytes, err := e.Bytes()
		if err != nil {
			// maybe I need to keep track of whether there were any
			// errors and at the end if there were, decline to
			// actually send anything (unless a force flag was set).
			logger.Print(err)
			break Loop
		}
		logger.Printf("[%v]", string(emailBytes))

		auth := smtp.PlainAuth("", *smtpUser, *smtpPassword, smtpHost)

		tokens := []string{smtpHost, smtpPort}
		err = e.Send(strings.Join(tokens, ":"), auth)
		if err != nil {
			// maybe I need to keep track of whether there were any
			// errors and at the end if there were, decline to
			// actually send anything (unless a force flag was set).
			logger.Print(err)
			break Loop
		}

		// next step: make a valid message!
		b, err := e.Bytes()
		if err != nil {
			log.Fatal(err)
		}
		literal := imap.NewLiteral(b)
		if _, err := imap.Wait(c.Append(*imapSent, imap.NewFlagSet("\\Seen"), nil, literal)); err != nil {
			log.Print(err)
		}
		log.Print("saved test message")
	}

}
