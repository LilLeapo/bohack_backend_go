package mailer

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/mail"
	"net/smtp"
	"strings"
)

type Mailer interface {
	SendVerificationCode(ctx context.Context, email, code, codeType string) error
	SendAttendanceConfirmation(ctx context.Context, email, name, eventTitle, confirmURL, declineURL string) error
	Mode() string
}

type ConsoleMailer struct{}

func NewConsoleMailer() Mailer {
	return &ConsoleMailer{}
}

func (m *ConsoleMailer) SendVerificationCode(_ context.Context, email, code, codeType string) error {
	log.Printf("[mail:console] send verification code email=%s type=%s code=%s", email, codeType, code)
	return nil
}

func (m *ConsoleMailer) SendAttendanceConfirmation(_ context.Context, email, name, eventTitle, confirmURL, declineURL string) error {
	log.Printf("[mail:console] send attendance confirmation email=%s name=%s event=%s confirm=%s decline=%s", email, name, eventTitle, confirmURL, declineURL)
	return nil
}

func (m *ConsoleMailer) Mode() string {
	return "console"
}

type SMTPMailer struct {
	host     string
	port     int
	username string
	password string
	from     string
}

func NewSMTPMailer(host string, port int, username, password, from string) Mailer {
	return &SMTPMailer{
		host:     strings.TrimSpace(host),
		port:     port,
		username: strings.TrimSpace(username),
		password: password,
		from:     strings.TrimSpace(from),
	}
}

func (m *SMTPMailer) SendVerificationCode(_ context.Context, email, code, codeType string) error {
	subject := "BoHack verification code"
	purpose := "verification"
	switch strings.TrimSpace(strings.ToLower(codeType)) {
	case "register":
		purpose = "registration"
	case "reset":
		purpose = "password reset"
	}

	body := fmt.Sprintf(
		"Your BoHack %s code is: %s\n\nIf you did not request this code, you can ignore this email.\n",
		purpose,
		code,
	)

	return m.send(email, subject, body)
}

func (m *SMTPMailer) SendAttendanceConfirmation(_ context.Context, email, name, eventTitle, confirmURL, declineURL string) error {
	subject := "BoHack attendance confirmation"
	if strings.TrimSpace(eventTitle) != "" {
		subject = eventTitle + " attendance confirmation"
	}
	if strings.TrimSpace(name) == "" {
		name = "there"
	}

	body := fmt.Sprintf(
		"Hi %s,\n\nPlease confirm whether you can attend %s.\n\nConfirm attendance:\n%s\n\nUnable to attend:\n%s\n\nIf you did not expect this email, you can ignore it.\n",
		name,
		eventTitle,
		confirmURL,
		declineURL,
	)

	return m.send(email, subject, body)
}

func (m *SMTPMailer) send(to, subject, body string) error {
	message := strings.Join([]string{
		"From: " + m.from,
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		body,
	}, "\r\n")

	addr := fmt.Sprintf("%s:%d", m.host, m.port)
	from := m.envelopeFrom()
	recipients := []string{to}

	if m.port == 465 {
		return m.sendImplicitTLS(addr, from, recipients, []byte(message))
	}

	return smtp.SendMail(addr, m.auth(), from, recipients, []byte(message))
}

func (m *SMTPMailer) sendImplicitTLS(addr, from string, to []string, message []byte) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: m.host,
	})
	if err != nil {
		return err
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, m.host)
	if err != nil {
		return err
	}
	defer client.Close()

	if auth := m.implicitTLSAuth(); auth != nil {
		if err := client.Auth(auth); err != nil {
			return err
		}
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	for _, recipient := range to {
		if err := client.Rcpt(recipient); err != nil {
			return err
		}
	}

	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(message); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

	return client.Quit()
}

func (m *SMTPMailer) auth() smtp.Auth {
	if m.username == "" {
		return nil
	}
	return smtp.PlainAuth("", m.username, m.password, m.host)
}

func (m *SMTPMailer) implicitTLSAuth() smtp.Auth {
	if m.username == "" {
		return nil
	}
	return implicitTLSPlainAuth{username: m.username, password: m.password}
}

func (m *SMTPMailer) envelopeFrom() string {
	address, err := mail.ParseAddress(m.from)
	if err != nil {
		return m.from
	}
	return address.Address
}

type implicitTLSPlainAuth struct {
	username string
	password string
}

func (a implicitTLSPlainAuth) Start(_ *smtp.ServerInfo) (string, []byte, error) {
	return "PLAIN", []byte("\x00" + a.username + "\x00" + a.password), nil
}

func (a implicitTLSPlainAuth) Next(_ []byte, more bool) ([]byte, error) {
	if more {
		return nil, fmt.Errorf("unexpected server challenge")
	}
	return nil, nil
}

func (m *SMTPMailer) Mode() string {
	return "smtp"
}
