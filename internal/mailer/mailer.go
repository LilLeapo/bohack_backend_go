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

type emailMessage struct {
	subject string
	text    string
	html    string
}

type verificationEmailData struct {
	Subject      string
	Eyebrow      string
	Title        string
	Lead         string
	Code         string
	Action       string
	FooterNote   string
	IgnoreNotice string
}

type attendanceEmailData struct {
	Subject    string
	Name       string
	EventTitle string
	ConfirmURL string
	DeclineURL string
}

var verificationEmailTemplate = template.Must(template.New("verification-email").Parse(`
<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Subject}}</title>
</head>
<body style="margin:0;background:#241f1a;color:#f7f1e5;font-family:'Space Grotesk','Noto Sans SC','PingFang SC','Microsoft YaHei',Arial,sans-serif;">
  <div style="display:none;overflow:hidden;line-height:1px;opacity:0;max-height:0;max-width:0;">{{.Lead}}</div>
  <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="background:#241f1a;">
    <tr>
      <td align="center" style="padding:36px 16px;">
        <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="max-width:640px;background:#fbf7ed;color:#241f1a;border-radius:28px;overflow:hidden;border:1px solid rgba(247,241,229,0.18);">
          <tr>
            <td style="padding:28px 28px 20px;background:#241f1a;color:#f7f1e5;">
              <div style="font-size:12px;letter-spacing:0.16em;text-transform:uppercase;color:#cff65d;">BOHACK / 2026</div>
              <div style="margin-top:42px;font-size:13px;letter-spacing:0.12em;text-transform:uppercase;color:rgba(247,241,229,0.72);">{{.Eyebrow}}</div>
              <h1 style="margin:12px 0 0;font-size:48px;line-height:0.95;letter-spacing:-0.05em;font-weight:800;">{{.Title}}</h1>
            </td>
          </tr>
          <tr>
            <td style="padding:32px 28px 30px;">
              <p style="margin:0;color:#4b4036;font-size:17px;line-height:1.7;">{{.Lead}}</p>
              <div style="margin:30px 0;padding:26px;border-radius:24px;background:#241f1a;color:#f7f1e5;text-align:center;">
                <div style="font-size:12px;letter-spacing:0.18em;text-transform:uppercase;color:rgba(247,241,229,0.62);">{{.Action}}</div>
                <div style="margin-top:14px;font-family:'JetBrains Mono','SFMono-Regular',Consolas,monospace;font-size:44px;line-height:1;letter-spacing:0.22em;font-weight:800;color:#cff65d;">{{.Code}}</div>
              </div>
              <div style="display:grid;gap:10px;margin-top:20px;">
                <p style="margin:0;color:#4b4036;font-size:15px;line-height:1.7;">{{.FooterNote}}</p>
                <p style="margin:0;color:#8b8177;font-size:13px;line-height:1.7;">{{.IgnoreNotice}}</p>
              </div>
              <div style="margin-top:30px;padding-top:18px;border-top:1px solid rgba(36,31,26,0.12);font-family:'JetBrains Mono','SFMono-Regular',Consolas,monospace;font-size:12px;letter-spacing:0.08em;color:#6a5f55;">
                天津 / 2026.05.22-31 · WIE 2026 OFFICIAL TRACK
              </div>
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`))

var attendanceEmailTemplate = template.Must(template.New("attendance-email").Parse(`
<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Subject}}</title>
</head>
<body style="margin:0;background:#241f1a;color:#f7f1e5;font-family:'Space Grotesk','Noto Sans SC','PingFang SC','Microsoft YaHei',Arial,sans-serif;">
  <div style="display:none;overflow:hidden;line-height:1px;opacity:0;max-height:0;max-width:0;">请确认你是否可以参加 BOHACK 2026 线下黑客松。</div>
  <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="background:#241f1a;">
    <tr>
      <td align="center" style="padding:36px 16px;">
        <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="max-width:680px;background:#fbf7ed;color:#241f1a;border-radius:28px;overflow:hidden;border:1px solid rgba(247,241,229,0.18);">
          <tr>
            <td style="padding:30px 30px 26px;background:#241f1a;color:#f7f1e5;">
              <div style="font-size:12px;letter-spacing:0.16em;text-transform:uppercase;color:#cff65d;">BOHACK / 2026</div>
              <div style="margin-top:42px;font-size:13px;letter-spacing:0.12em;text-transform:uppercase;color:rgba(247,241,229,0.72);">Attendance Confirmation · 参赛时间确认</div>
              <h1 style="margin:12px 0 0;font-size:46px;line-height:0.98;letter-spacing:-0.05em;font-weight:800;">确认你的参赛时间。</h1>
            </td>
          </tr>
          <tr>
            <td style="padding:32px 30px 30px;">
              <p style="margin:0;color:#4b4036;font-size:17px;line-height:1.7;">{{.Name}}，你好。你已进入 {{.EventTitle}} 参赛时间确认环节，请选择是否可以参加线下活动，方便主办方安排名额、签到和现场资源。</p>
              <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="margin:28px 0;border-collapse:separate;border-spacing:0 12px;">
                <tr>
                  <td style="padding:16px 18px;border-radius:18px;background:#f0eadc;color:#241f1a;font-size:15px;line-height:1.6;">
                    <strong style="font-family:'JetBrains Mono','SFMono-Regular',Consolas,monospace;">5月22日 16:00</strong><br>
                    线下签到入场 · 天开高教科创园
                  </td>
                </tr>
                <tr>
                  <td style="padding:16px 18px;border-radius:18px;background:#f0eadc;color:#241f1a;font-size:15px;line-height:1.6;">
                    <strong style="font-family:'JetBrains Mono','SFMono-Regular',Consolas,monospace;">5月22日—5月24日</strong><br>
                    42小时线下黑客松
                  </td>
                </tr>
                <tr>
                  <td style="padding:16px 18px;border-radius:18px;background:#f0eadc;color:#241f1a;font-size:15px;line-height:1.6;">
                    <strong style="font-family:'JetBrains Mono','SFMono-Regular',Consolas,monospace;">5月24日—5月31日</strong><br>
                    线上项目打磨与世界智能产业博览会现场展演
                  </td>
                </tr>
              </table>
              <table role="presentation" cellspacing="0" cellpadding="0" style="width:100%;">
                <tr>
                  <td style="padding:0 0 12px;">
                    <a href="{{.ConfirmURL}}" style="display:block;padding:17px 22px;border-radius:999px;background:#cff65d;color:#241f1a;text-align:center;text-decoration:none;font-weight:800;">确认参加 ↗</a>
                  </td>
                </tr>
                <tr>
                  <td>
                    <a href="{{.DeclineURL}}" style="display:block;padding:15px 22px;border-radius:999px;background:#241f1a;color:#f7f1e5;text-align:center;text-decoration:none;font-weight:700;">暂时无法参加</a>
                  </td>
                </tr>
              </table>
              <p style="margin:22px 0 0;color:#8b8177;font-size:13px;line-height:1.7;">如果按钮无法打开，请复制对应链接到浏览器。若你没有预期收到这封邮件，可以忽略它。</p>
              <div style="margin-top:30px;padding-top:18px;border-top:1px solid rgba(36,31,26,0.12);font-family:'JetBrains Mono','SFMono-Regular',Consolas,monospace;font-size:12px;letter-spacing:0.08em;color:#6a5f55;">
                天津 / 2026.05.22-31 · WIE 2026 OFFICIAL TRACK
              </div>
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`))

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

func buildVerificationCodeEmail(code, codeType string) (emailMessage, error) {
	code = strings.TrimSpace(code)
	data := verificationEmailData{
		Subject:      "BOHACK 2026 注册验证码",
		Eyebrow:      "邮箱验证 · 账号注册",
		Title:        "创建账号。",
		Lead:         "你正在注册 BOHACK 2026 平台账号。账号用于保存报名问卷、查看审核状态，并接收活动相关通知。",
		Code:         code,
		Action:       "Registration Code",
		FooterNote:   "请在验证码有效期内完成验证。账号创建后，可以继续填写报名问卷，问卷审核通过后才会成为正式选手身份。",
		IgnoreNotice: "如果不是你本人操作，可以忽略本邮件。",
	}

	switch strings.TrimSpace(strings.ToLower(codeType)) {
	case "reset":
		data.Subject = "BOHACK 2026 密码重置验证码"
		data.Eyebrow = "邮箱验证 · 密码重置"
		data.Title = "重置密码。"
		data.Lead = "你正在重置 BOHACK 2026 平台账号密码。请使用下方验证码完成身份验证。"
		data.Action = "Reset Code"
		data.FooterNote = "请在验证码有效期内完成验证。完成后，你可以使用新密码登录 BOHACK 平台。"
	case "register", "":
	default:
		data.Subject = "BOHACK 2026 验证码"
		data.Eyebrow = "邮箱验证"
		data.Title = "完成验证。"
		data.Lead = "你正在进行 BOHACK 2026 平台邮箱验证。请使用下方验证码完成操作。"
		data.Action = "Verification Code"
		data.FooterNote = "请在验证码有效期内完成验证。"
	}

	html, err := renderTemplate(verificationEmailTemplate, data)
	if err != nil {
		return emailMessage{}, err
	}

	text := fmt.Sprintf("%s\n\n%s\n\n验证码：%s\n\n%s\n\n%s\n\nBOHACK 2026 · 天津 / 2026.05.22-31 · WIE 2026 OFFICIAL TRACK\n",
		data.Subject,
		data.Lead,
		data.Code,
		data.FooterNote,
		data.IgnoreNotice,
	)

	return emailMessage{
		subject: data.Subject,
		text:    text,
		html:    html,
	}, nil
}

func buildAttendanceConfirmationEmail(name, eventTitle, confirmURL, declineURL string) (emailMessage, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "同学"
	}
	eventTitle = strings.TrimSpace(eventTitle)
	if eventTitle == "" {
		eventTitle = "BOHACK 2026"
	}

	data := attendanceEmailData{
		Subject:    eventTitle + " 参赛时间确认",
		Name:       name,
		EventTitle: eventTitle,
		ConfirmURL: strings.TrimSpace(confirmURL),
		DeclineURL: strings.TrimSpace(declineURL),
	}

	html, err := renderTemplate(attendanceEmailTemplate, data)
	if err != nil {
		return emailMessage{}, err
	}

	text := fmt.Sprintf(`%s

%s，你好。

你已进入 %s 参赛时间确认环节，请选择是否可以参加线下活动，方便主办方安排名额、签到和现场资源。

关键时间：
- 5月22日 16:00：线下签到入场 · 天开高教科创园
- 5月22日—5月24日：42小时线下黑客松
- 5月24日—5月31日：线上项目打磨与世界智能产业博览会现场展演

确认参加：
%s

暂时无法参加：
%s

如果不是你本人操作，可以忽略本邮件。
BOHACK 2026 · 天津 / 2026.05.22-31 · WIE 2026 OFFICIAL TRACK
`,
		data.Subject,
		data.Name,
		data.EventTitle,
		data.ConfirmURL,
		data.DeclineURL,
	)

	return emailMessage{
		subject: data.Subject,
		text:    text,
		html:    html,
	}, nil
}

func renderTemplate(tpl *template.Template, data any) (string, error) {
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return strings.TrimSpace(buf.String()), nil
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

func encodeHeader(value string) string {
	return mime.QEncoding.Encode("UTF-8", cleanHeader(value))
}

func cleanHeader(value string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(strings.TrimSpace(value))
}

func encodeQuotedPrintable(value string) string {
	var buf bytes.Buffer
	writer := quotedprintable.NewWriter(&buf)
	_, _ = writer.Write([]byte(value))
	_ = writer.Close()
	return buf.String()
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
