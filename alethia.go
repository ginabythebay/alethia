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

type Context struct {
	tabularFileName string
	imapClient      *Client
	headers         *template.Template
	body            *template.Template
	sender          *Sender
}

func (c *Context) newEmail(templateValues interface{}) (*OurEmail, error) {
	headerText, err := executeTemplate(c.headers, templateValues)
	if err != nil {
		return nil, err
	}
	rr := textproto.NewReader(bufio.NewReader(strings.NewReader(headerText)))
	mimeHeader, err := rr.ReadMIMEHeader()
	if err != nil && err != io.EOF {
		return nil, err
	}

	bodyText, err := executeTemplate(c.body, templateValues)
	if err != nil {
		return nil, err
	}

	return c.sender.NewEmail(mimeHeader, bodyText), nil
}

func (c *Context) process() (processedCount int, rowErrorCount int, fatal error) {
	tabularFile, err := os.Open(c.tabularFileName)
	if err != nil {
		return 0, 0, err
	}
	defer tabularFile.Close()

	tabularReader, err := NewTabularReader(tabularFile)
	if err != nil {
		return 0, 0, err
	}

	processedCount = 0
	rowErrorCount = 0

	for {
		record, err := tabularReader.Read()
		switch {
		case err == io.EOF:
			return processedCount, rowErrorCount, nil
		case err != nil:
			return processedCount, rowErrorCount, err
		}
		templateValues := combine(record, namedValueFlag)

		e, err := c.newEmail(templateValues)
		if err != nil {
			logger.Print(err)
			rowErrorCount += 1
		}

		err = e.Send()
		if err != nil {
			logger.Print(err)
			rowErrorCount += 1
		}

		// Save sent message to imap folder
		if c.imapClient != nil {
			b, err := e.Bytes()
			if err != nil {
				return processedCount, rowErrorCount, err
			}
			c.imapClient.Save(b)
		}

		processedCount += 1
	}
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

	var imapClient *Client = nil
	if !*skipImap {
		imapClient, err = Dial(*imapServer, *insecureSkipVerify, *imapUser, *imapPassword, *imapSent)
		if err != nil {
			log.Fatal(err)
		}

		defer imapClient.Logout(30 * time.Second)
	}

	sender := NewSender(smtpHost, smtpPort, *smtpUser, *smtpPassword)

	context := Context{*tabularFileName, imapClient, headersTemplate, bodyTemplate, sender}

	// need to learn the loop into a method on context, where we open
	// up the reader, loop through records should return an error
	// count, and a possible error can config it with verbosity,
	// whether to actually send (typical flow will be dry run followed
	// by real run)

	processedCount, rowErrorCount, fatalError := context.process()
	if fatalError != nil {
		log.Fatal(fatalError)
	}
	log.Printf("Processed %v with %v successes and %v failures", context.tabularFileName, processedCount, rowErrorCount)
}
