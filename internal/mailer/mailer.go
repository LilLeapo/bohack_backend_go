package mailer

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"html/template"
	"log"
	"mime"
	"mime/quotedprintable"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
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
	log.Printf("[mail:console] accepted verification code email=%s type=%s", maskEmail(email), codeType)
	return nil
}

func (m *ConsoleMailer) SendAttendanceConfirmation(_ context.Context, email, name, eventTitle, confirmURL, declineURL string) error {
	log.Printf("[mail:console] accepted attendance confirmation email=%s name=%s event=%s", maskEmail(email), name, eventTitle)
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
	message, err := buildVerificationCodeEmail(code, codeType)
	if err != nil {
		return err
	}

	log.Printf("[mail:smtp] sending verification code email=%s type=%s host=%s port=%d", maskEmail(email), codeType, m.host, m.port)
	if err := m.send(email, message); err != nil {
		log.Printf("[mail:smtp] send verification code failed email=%s type=%s host=%s port=%d err=%v", maskEmail(email), codeType, m.host, m.port, err)
		return err
	}
	log.Printf("[mail:smtp] send verification code accepted email=%s type=%s host=%s port=%d", maskEmail(email), codeType, m.host, m.port)
	return nil
}

func (m *SMTPMailer) SendAttendanceConfirmation(_ context.Context, email, name, eventTitle, confirmURL, declineURL string) error {
	message, err := buildAttendanceConfirmationEmail(name, eventTitle, confirmURL, declineURL)
	if err != nil {
		return err
	}

	log.Printf("[mail:smtp] sending attendance confirmation email=%s event=%s host=%s port=%d", maskEmail(email), eventTitle, m.host, m.port)
	if err := m.send(email, message); err != nil {
		log.Printf("[mail:smtp] send attendance confirmation failed email=%s event=%s host=%s port=%d err=%v", maskEmail(email), eventTitle, m.host, m.port, err)
		return err
	}
	log.Printf("[mail:smtp] send attendance confirmation accepted email=%s event=%s host=%s port=%d", maskEmail(email), eventTitle, m.host, m.port)
	return nil
}

func (m *SMTPMailer) send(to string, message emailMessage) error {
	boundary := fmt.Sprintf("bohack-%d", time.Now().UnixNano())
	raw := strings.Join([]string{
		"From: " + m.from,
		"To: " + to,
		"Subject: " + encodeHeader(message.subject),
		"MIME-Version: 1.0",
		"Content-Type: multipart/alternative; boundary=\"" + boundary + "\"",
		"",
		"--" + boundary,
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Transfer-Encoding: quoted-printable",
		"",
		encodeQuotedPrintable(message.text),
		"--" + boundary,
		"Content-Type: text/html; charset=UTF-8",
		"Content-Transfer-Encoding: quoted-printable",
		"",
		encodeQuotedPrintable(message.html),
		"--" + boundary + "--",
		"",
	}, "\r\n")

	addr := fmt.Sprintf("%s:%d", m.host, m.port)
	from := m.envelopeFrom()
	recipients := []string{to}

	if m.port == 465 {
		return m.sendImplicitTLS(addr, from, recipients, []byte(raw))
	}

	return smtp.SendMail(addr, m.auth(), from, recipients, []byte(raw))
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

func maskEmail(email string) string {
	email = strings.TrimSpace(email)
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return email
	}
	local := parts[0]
	if len(local) <= 2 {
		return strings.Repeat("*", len(local)) + "@" + parts[1]
	}
	return local[:1] + strings.Repeat("*", len(local)-2) + local[len(local)-1:] + "@" + parts[1]
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
