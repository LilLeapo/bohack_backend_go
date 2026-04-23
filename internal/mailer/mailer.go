package mailer

import (
	"context"
	"fmt"
	"log"
	"net/smtp"
	"strings"
)

type Mailer interface {
	SendVerificationCode(ctx context.Context, email, code, codeType string) error
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

	message := strings.Join([]string{
		"From: " + m.from,
		"To: " + email,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		body,
	}, "\r\n")

	addr := fmt.Sprintf("%s:%d", m.host, m.port)
	var auth smtp.Auth
	if m.username != "" {
		auth = smtp.PlainAuth("", m.username, m.password, m.host)
	}

	return smtp.SendMail(addr, auth, m.from, []string{email}, []byte(message))
}

func (m *SMTPMailer) Mode() string {
	return "smtp"
}
