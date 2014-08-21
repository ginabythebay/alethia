package main

import (
	"bufio"
	"bytes"
	"flag"
	"io"
	"log"
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
	// smtp flags
	smtpServer := flag.String("smtpServer", "", "name (or ip) of smtp server (host:port)")
	smtpUser := flag.String("smtpUser", "", "user for smtp log in")
	smtpPassword := flag.String("smtpPassword", "", "password for smtp log in")

	// imap flags
	skipImap := flag.Bool("skipImap", false, "If set, disables saving to imap")
	imapServer := flag.String("imapServer", "", "name (or ip) of imap server.  May be left blank if same as host of smtpServer.")
	imapUser := flag.String("imapUser", "", "user for imap log in.  May be left blank if same as smtpUse.r")
	imapPassword := flag.String("imapPassword", "", "password for imap log in.  May be left blank if same as smtpPassword.")
	imapSent := flag.String("imapSent", "INBOX.Sent", "Folder on imap server for sent mail")
	insecureSkipVerify := flag.Bool("insecureSkipVerify", false, "If set, disables imap checking of hosts certificate chain and host name (needed with some dreamhost servers because they use a single certificate for all mail hosts")

	// message input flags
	templateFile := flag.String("templateFile", "", "Name of file containing template")
	tabularFileName := flag.String("tabularFile", "", "Name of tsv or csv file (one row per email)")
	flag.Var(namedValueFlag, "nv", "key=value pair that will be used in template.  This flag may be specified multiple times")

	flag.Parse()

	smtpHost, smtpPort := parseSmtpServer(*smtpServer)

	if len(*imapServer) == 0 {
		imapServer = &smtpHost
	}
	if len(*imapUser) == 0 {
		imapUser = smtpUser
	}
	if len(*imapPassword) == 0 {
		imapPassword = smtpPassword
	}

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

	var c *Client = nil
	if !*skipImap {
		c, err = Dial(*imapServer, *insecureSkipVerify, *imapUser, *imapPassword, *imapSent)
		if err != nil {
			log.Fatal(err)
		}

		defer c.Logout(30 * time.Second)
	}

	sender := NewSender(smtpHost, smtpPort, *smtpUser, *smtpPassword)

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

		e := sender.NewEmail(mimeHeader, bodyText)

		err = e.Send()
		if err != nil {
			// maybe I need to keep track of whether there were any
			// errors and at the end if there were, decline to
			// actually send anything (unless a force flag was set).
			logger.Print(err)
			break Loop
		}

		// Save sent message to imap folder
		if c != nil {
			b, err := e.Bytes()
			if err != nil {
				log.Fatal(err)
			}
			c.Save(b)
		}
	}

}
