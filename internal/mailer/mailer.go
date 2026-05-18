package mailer

import (
	"bytes"
	"context"
	"crypto/tls"
	"embed"
	"encoding/base64"
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
	SendAttendanceConfirmation(ctx context.Context, email, name, eventTitle, confirmURL, declineURL string, sentAt, confirmationDeadline time.Time) error
	SendRegistrationEmail(ctx context.Context, email string, params RegistrationEmailParams) error
	Mode() string
}

const (
	RegistrationEmailAdmission         RegistrationEmailKind = "admission"
	RegistrationEmailVisitor           RegistrationEmailKind = "visitor"
	RegistrationEmailMinorAdmission    RegistrationEmailKind = "minor_admission"
	RegistrationEmailAgreementReminder RegistrationEmailKind = "agreement_reminder"

	helperQRCodeContentID = "bohack-helper-qr"
	helperQRCodeFilename  = "xiaozhushou-wxqr.png"
	riskConfirmationFile  = "2026智能创新黑客松活动风险告知与参与确认书.pdf"
)

//go:embed assets/*
var emailAssets embed.FS

type RegistrationEmailKind string

type RegistrationEmailParams struct {
	Kind                 RegistrationEmailKind
	Name                 string
	ConfirmURL           string
	SentAt               time.Time
	ConfirmationDeadline time.Time
}

type EmailPreview struct {
	Subject     string
	Text        string
	HTML        string
	Attachments []string
}

func BuildRegistrationEmailPreview(params RegistrationEmailParams) (EmailPreview, error) {
	message, err := buildRegistrationEmail(params)
	if err != nil {
		return EmailPreview{}, err
	}

	attachments := make([]string, 0, len(message.parts))
	for _, part := range message.parts {
		if part.inline {
			continue
		}
		attachments = append(attachments, part.filename)
	}

	return EmailPreview{
		Subject:     message.subject,
		Text:        message.text,
		HTML:        message.html,
		Attachments: attachments,
	}, nil
}

func ParseRegistrationEmailKind(value string) (RegistrationEmailKind, bool) {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "admission", "accepted", "attendance_confirmation", "attendance-confirmation":
		return RegistrationEmailAdmission, true
	case "visitor", "experience", "experiencer":
		return RegistrationEmailVisitor, true
	case "minor_admission", "minor-admission", "minor":
		return RegistrationEmailMinorAdmission, true
	case "agreement_reminder", "agreement-reminder", "unsigned_agreement", "unsigned-agreement":
		return RegistrationEmailAgreementReminder, true
	default:
		return "", false
	}
}

func (k RegistrationEmailKind) RequiresConfirmURL() bool {
	return k == RegistrationEmailAdmission || k == RegistrationEmailMinorAdmission
}

type ConsoleMailer struct{}

func NewConsoleMailer() Mailer {
	return &ConsoleMailer{}
}

func (m *ConsoleMailer) SendVerificationCode(_ context.Context, email, code, codeType string) error {
	log.Printf("[mail:console] accepted verification code email=%s type=%s", maskEmail(email), codeType)
	return nil
}

func (m *ConsoleMailer) SendAttendanceConfirmation(_ context.Context, email, name, eventTitle, confirmURL, declineURL string, sentAt, confirmationDeadline time.Time) error {
	log.Printf("[mail:console] accepted attendance confirmation email=%s name=%s event=%s", maskEmail(email), name, eventTitle)
	return nil
}

func (m *ConsoleMailer) SendRegistrationEmail(_ context.Context, email string, params RegistrationEmailParams) error {
	log.Printf("[mail:console] accepted registration email=%s kind=%s name=%s", maskEmail(email), params.Kind, params.Name)
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
	parts   []emailPart
}

type emailPart struct {
	filename    string
	contentType string
	contentID   string
	inline      bool
	data        []byte
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
	Subject          string
	Name             string
	ConfirmURL       string
	DeadlineText     string
	SentDateText     string
	QRCodeContentID  string
	AttachmentName   string
	AttachmentNotice string
}

type registrationCopyEmailData struct {
	Subject         string
	Preheader       string
	Eyebrow         string
	HeroTitle       string
	HeroSubtitle    string
	BodyHTML        template.HTML
	QRCodeContentID string
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
  <div style="display:none;overflow:hidden;line-height:1px;opacity:0;max-height:0;max-width:0;">欢迎正式加入2026智能创新黑客松，请在{{.DeadlineText}}确认参赛。</div>
  <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="background:#241f1a;">
    <tr>
      <td align="center" style="padding:36px 16px;">
        <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="max-width:680px;background:#fbf7ed;color:#241f1a;border-radius:28px;overflow:hidden;border:1px solid rgba(247,241,229,0.18);">
          <tr>
            <td style="padding:30px 30px 28px;background:#241f1a;color:#f7f1e5;">
              <div style="font-size:12px;letter-spacing:0.16em;text-transform:uppercase;color:#cff65d;">BOHACK / 2026</div>
              <div style="margin-top:42px;font-size:13px;letter-spacing:0.12em;text-transform:uppercase;color:rgba(247,241,229,0.72);">Attendance Confirmation · 参赛时间确认</div>
              <h1 style="margin:12px 0 0;font-size:44px;line-height:1.02;font-weight:800;">您已获得参赛席位。</h1>
              <p style="margin:16px 0 0;color:rgba(247,241,229,0.78);font-size:15px;line-height:1.7;">2026智能创新黑客松正式录取通知</p>
            </td>
          </tr>
          <tr>
            <td style="padding:32px 30px 34px;">
              <p style="margin:0 0 18px;color:#241f1a;font-size:18px;line-height:1.7;font-weight:800;">{{.Name}}BoHacker：</p>
              <p style="margin:0;color:#4b4036;font-size:16px;line-height:1.85;">您好！经过仔细的评估与筛选，我们非常荣幸地欢迎您正式加入2026智能创新黑客松！您在报名材料中所展现出的技术热情、独特思考，以及在过往项目经历中迸发的创造力，让我们坚信，您正是我们一直在寻找的“造物者”。在这里，您将与志同道合的伙伴一起，在42小时内将创意变为现实，还将获得深度的项目赋能，最终在2026世界智能产业博览会的舞台上，与产业巨头同台，在主流媒体与投资机构的注视下，让您的作品绽放光芒。</p>

              <p style="margin:22px 0 0;color:#4b4036;font-size:15px;line-height:1.75;">请您仔细阅读以下重要信息，并做好相应准备：</p>

              <div style="margin:28px 0 14px;font-size:13px;letter-spacing:0.12em;text-transform:uppercase;color:#7c6f63;font-weight:800;">一、活动核心信息</div>
              <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="border-collapse:separate;border-spacing:0 12px;">
                <tr>
                  <td style="width:112px;padding:16px 18px;border-radius:18px 0 0 18px;background:#f0eadc;color:#7c6f63;font-size:14px;line-height:1.6;font-weight:800;">活动名称</td>
                  <td style="padding:16px 18px;border-radius:0 18px 18px 0;background:#f0eadc;color:#241f1a;font-size:15px;line-height:1.6;">2026智能创新黑客松</td>
                </tr>
                <tr>
                  <td style="width:112px;padding:16px 18px;border-radius:18px 0 0 18px;background:#f0eadc;color:#7c6f63;font-size:14px;line-height:1.6;font-weight:800;">报到时间</td>
                  <td style="padding:16px 18px;border-radius:0 18px 18px 0;background:#f0eadc;color:#241f1a;font-size:15px;line-height:1.6;"><strong style="font-family:'JetBrains Mono','SFMono-Regular',Consolas,monospace;">5月22日（周五）16:00</strong></td>
                </tr>
              </table>

              <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="margin-top:4px;border-collapse:separate;border-spacing:0 12px;">
                <tr>
                  <td style="padding:16px 18px;border-radius:18px;background:#241f1a;color:#f7f1e5;font-size:15px;line-height:1.65;">
                    <strong style="font-family:'JetBrains Mono','SFMono-Regular',Consolas,monospace;color:#cff65d;">5月22日——5月24日</strong><br>
                    42h线下黑客松 · 天开高教科创园
                  </td>
                </tr>
                <tr>
                  <td style="padding:16px 18px;border-radius:18px;background:#241f1a;color:#f7f1e5;font-size:15px;line-height:1.65;">
                    <strong style="font-family:'JetBrains Mono','SFMono-Regular',Consolas,monospace;color:#cff65d;">5月25日——5月27日</strong><br>
                    项目赋能 · 线上
                  </td>
                </tr>
                <tr>
                  <td style="padding:16px 18px;border-radius:18px;background:#241f1a;color:#f7f1e5;font-size:15px;line-height:1.65;">
                    <strong style="font-family:'JetBrains Mono','SFMono-Regular',Consolas,monospace;color:#cff65d;">5月28日——5月31日</strong><br>
                    展览+路演 · 国家会展中心
                  </td>
                </tr>
              </table>

              <div style="margin:30px 0 14px;font-size:13px;letter-spacing:0.12em;text-transform:uppercase;color:#7c6f63;font-weight:800;">二、后续步骤与须知</div>
              <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="border-collapse:separate;border-spacing:0 14px;">
                <tr>
                  <td style="padding:18px 18px 20px;border-radius:20px;background:#fffaf0;color:#4b4036;font-size:15px;line-height:1.75;border:1px solid rgba(36,31,26,0.08);">
                    <div style="margin-bottom:8px;color:#241f1a;font-weight:800;">1. 确认参赛</div>
                    请务必于<strong>{{.DeadlineText}}</strong>点击下方链接确认参赛，并上传手写签署后的《2026智能创新黑客松活动风险告知与参与确认书》（见附件），以确认您的参赛资格。如因个人安排变动无法如期参与，请回复本邮件告知<strong>【姓名 + 无法参赛 + 简要原因】</strong>，我们将尽力为您协调或提供协助。逾期未确认将视为自动放弃。
                    <a href="{{.ConfirmURL}}" style="display:block;margin-top:16px;padding:16px 22px;border-radius:999px;background:#cff65d;color:#241f1a;text-align:center;text-decoration:none;font-weight:800;">确认参赛并上传确认书 ↗</a>
                  </td>
                </tr>
                <tr>
                  <td style="padding:18px;border-radius:20px;background:#fffaf0;color:#4b4036;font-size:15px;line-height:1.75;border:1px solid rgba(36,31,26,0.08);">
                    <div style="margin-bottom:8px;color:#241f1a;font-weight:800;">2. 添加小助手微信</div>
                    为确保信息畅通，请使用微信扫描以下二维码或搜索“15522512264”添加BoHack官方小助手，添加时请将昵称修改为“2026智能创新黑客松-姓名-学校/单位”，后续事项将通过该微信进行通知。
                    <img src="cid:{{.QRCodeContentID}}" alt="BoHack 小助手微信二维码" width="180" style="display:block;margin-top:14px;width:180px;max-width:100%;height:auto;border-radius:22px;border:1px solid rgba(36,31,26,0.12);background:#fffdf6;">
                  </td>
                </tr>
                <tr>
                  <td style="padding:18px;border-radius:20px;background:#fffaf0;color:#4b4036;font-size:15px;line-height:1.75;border:1px solid rgba(36,31,26,0.08);">
                    <div style="margin-bottom:8px;color:#241f1a;font-weight:800;">3. 活动详情</div>
                    关于活动的详细日程、规则、赛题发布等信息，请持续关注小助手微信消息及BoHack官方微信公众号。
                  </td>
                </tr>
              </table>

              <p style="margin:20px 0 0;color:#4b4036;font-size:15px;line-height:1.85;">九河下梢，海河之畔，创新的潮水正奔腾涌动。我们诚邀您，成为这浪潮中最激越的一脉；我们期待您，带着智慧的星火与不羁的创意而来，用42小时将奇思淬炼为真；我们更将与您一同，将璀璨的成果推上世界瞩目的舞台。天津已准备好见证您的光芒，世界亦是。</p>
              <p style="margin:18px 0 0;color:#4b4036;font-size:15px;line-height:1.75;">如有任何问题，欢迎随时回复本邮件或添加小助手微信咨询。</p>
              <p style="margin:18px 0 0;color:#7c6f63;font-size:13px;line-height:1.7;">{{.AttachmentNotice}}</p>
              <p style="margin:24px 0 0;color:#241f1a;font-size:15px;line-height:1.7;font-weight:800;">BoHack组委会<br>{{.SentDateText}}</p>
              <p style="margin:22px 0 0;color:#8b8177;font-size:13px;line-height:1.7;">如果按钮无法打开，请复制链接到浏览器：{{.ConfirmURL}}</p>
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

var registrationCopyEmailTemplate = template.Must(template.New("registration-copy-email").Parse(`
<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Subject}}</title>
</head>
<body style="margin:0;background:#241f1a;color:#f7f1e5;font-family:'Space Grotesk','Noto Sans SC','PingFang SC','Microsoft YaHei',Arial,sans-serif;">
  <div style="display:none;overflow:hidden;line-height:1px;opacity:0;max-height:0;max-width:0;">{{.Preheader}}</div>
  <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="background:#241f1a;">
    <tr>
      <td align="center" style="padding:36px 16px;">
        <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="max-width:680px;background:#fbf7ed;color:#241f1a;border-radius:28px;overflow:hidden;border:1px solid rgba(247,241,229,0.18);">
          <tr>
            <td style="padding:30px 30px 28px;background:#241f1a;color:#f7f1e5;">
              <div style="font-size:12px;letter-spacing:0.16em;text-transform:uppercase;color:#cff65d;">BOHACK / 2026</div>
              <div style="margin-top:42px;font-size:13px;letter-spacing:0.12em;text-transform:uppercase;color:rgba(247,241,229,0.72);">{{.Eyebrow}}</div>
              <h1 style="margin:12px 0 0;font-size:42px;line-height:1.04;font-weight:800;">{{.HeroTitle}}</h1>
              <p style="margin:16px 0 0;color:rgba(247,241,229,0.78);font-size:15px;line-height:1.7;">{{.HeroSubtitle}}</p>
            </td>
          </tr>
          <tr>
            <td style="padding:32px 30px 34px;color:#4b4036;font-size:15px;line-height:1.8;">
              {{.BodyHTML}}
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

func (m *SMTPMailer) SendAttendanceConfirmation(_ context.Context, email, name, eventTitle, confirmURL, declineURL string, sentAt, confirmationDeadline time.Time) error {
	message, err := buildAttendanceConfirmationEmail(name, eventTitle, confirmURL, declineURL, sentAt, confirmationDeadline)
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

func (m *SMTPMailer) SendRegistrationEmail(_ context.Context, email string, params RegistrationEmailParams) error {
	message, err := buildRegistrationEmail(params)
	if err != nil {
		return err
	}

	log.Printf("[mail:smtp] sending registration email=%s kind=%s host=%s port=%d", maskEmail(email), params.Kind, m.host, m.port)
	if err := m.send(email, message); err != nil {
		log.Printf("[mail:smtp] send registration email failed email=%s kind=%s host=%s port=%d err=%v", maskEmail(email), params.Kind, m.host, m.port, err)
		return err
	}
	log.Printf("[mail:smtp] send registration email accepted email=%s kind=%s host=%s port=%d", maskEmail(email), params.Kind, m.host, m.port)
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

func buildRegistrationEmail(params RegistrationEmailParams) (emailMessage, error) {
	switch params.Kind {
	case RegistrationEmailAdmission:
		return buildAttendanceConfirmationEmail(
			params.Name,
			"",
			params.ConfirmURL,
			"",
			params.SentAt,
			params.ConfirmationDeadline,
		)
	case RegistrationEmailVisitor:
		return buildVisitorEmail(params.Name, params.SentAt)
	case RegistrationEmailMinorAdmission:
		return buildMinorAdmissionEmail(params.Name, params.ConfirmURL, params.SentAt, params.ConfirmationDeadline)
	case RegistrationEmailAgreementReminder:
		return buildAgreementReminderEmail(params.Name)
	default:
		return emailMessage{}, fmt.Errorf("unsupported registration email kind %q", params.Kind)
	}
}

func buildAttendanceConfirmationEmail(name, eventTitle, confirmURL, declineURL string, sentAt, confirmationDeadline time.Time) (emailMessage, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "同学"
	}
	sentAt, confirmationDeadline = resolveConfirmationEmailTimes(sentAt, confirmationDeadline)

	data := attendanceEmailData{
		Subject:          "祝贺！您已正式获得2026智能创新黑客松的参赛席位！",
		Name:             name,
		ConfirmURL:       strings.TrimSpace(confirmURL),
		DeadlineText:     formatConfirmationDeadlineText(confirmationDeadline),
		SentDateText:     formatChineseDate(sentAt),
		QRCodeContentID:  helperQRCodeContentID,
		AttachmentName:   riskConfirmationFile,
		AttachmentNotice: "附件：" + riskConfirmationFile,
	}

	html, err := renderTemplate(attendanceEmailTemplate, data)
	if err != nil {
		return emailMessage{}, err
	}

	text := fmt.Sprintf(`%s

%sBoHacker：

您好！经过仔细的评估与筛选，我们非常荣幸地欢迎您正式加入2026智能创新黑客松！您在报名材料中所展现出的技术热情、独特思考，以及在过往项目经历中迸发的创造力，让我们坚信，您正是我们一直在寻找的“造物者”。在这里，您将与志同道合的伙伴一起，在42小时内将创意变为现实，还将获得深度的项目赋能，最终在2026世界智能产业博览会的舞台上，与产业巨头同台，在主流媒体与投资机构的注视下，让您的作品绽放光芒。

请您仔细阅读以下重要信息，并做好相应准备：

一、活动核心信息
- 活动名称：2026智能创新黑客松
- 活动时间与地点：
  - 42h线下黑客松：5月22日——5月24日（天开高教科创园）
  - 项目赋能：5月25日——5月27日（线上）
  - 展览+路演：5月28日——5月31日（国家会展中心）
- 报到时间：5月22日（周五）16:00

二、后续步骤与须知
1. 确认参赛：请务必于【%s】点击下方链接确认参赛，并上传手写签署后的《2026智能创新黑客松活动风险告知与参与确认书》（见附件），以确认您的参赛资格。如因个人安排变动无法如期参与，请回复本邮件告知【姓名 + 无法参赛 + 简要原因】，我们将尽力为您协调或提供协助。逾期未确认将视为自动放弃。
确认链接：%s
2. 添加小助手微信：为确保信息畅通，请使用微信扫描以下二维码或搜索“15522512264”添加BoHack官方小助手，添加时请将昵称修改为“2026智能创新黑客松-姓名-学校/单位”，后续事项将通过该微信进行通知。
（小助手二维码）
3. 活动详情：关于活动的详细日程、规则、赛题发布等信息，请持续关注小助手微信消息及BoHack官方微信公众号。

九河下梢，海河之畔，创新的潮水正奔腾涌动。我们诚邀您，成为这浪潮中最激越的一脉；我们期待您，带着智慧的星火与不羁的创意而来，用42小时将奇思淬炼为真；我们更将与您一同，将璀璨的成果推上世界瞩目的舞台。天津已准备好见证您的光芒，世界亦是。

如有任何问题，欢迎随时回复本邮件或添加小助手微信咨询。

BoHack组委会
%s
BOHACK 2026 · 天津 / 2026.05.22-31 · WIE 2026 OFFICIAL TRACK
`,
		data.Subject,
		data.Name,
		data.DeadlineText,
		data.ConfirmURL,
		data.SentDateText,
	)

	message := emailMessage{
		subject: data.Subject,
		text:    text,
		html:    html,
	}
	return addCommonRegistrationAssets(message, true)
}

func buildVisitorEmail(name string, sentAt time.Time) (emailMessage, error) {
	name = fallbackName(name)
	escapedName := template.HTMLEscapeString(name)
	if sentAt.IsZero() {
		sentAt = time.Now().UTC()
	}
	sentDateText := formatChineseDate(sentAt)
	subject := "关于2026智能创新黑客松大赛申请的回复通知"
	bodyHTML := template.HTML(fmt.Sprintf(`
              <p style="margin:0 0 18px;color:#241f1a;font-size:18px;line-height:1.7;font-weight:800;">%sBoHacker：</p>
              <p style="margin:0;color:#4b4036;font-size:15px;line-height:1.85;">您好！非常感谢您对2026智能创新黑客松大赛的关注与申请，也诚挚感谢您在申请过程中所付出的时间与思考。我们非常珍视每一位申请者的热情与创意。</p>
              <p style="margin:18px 0 0;color:#4b4036;font-size:15px;line-height:1.85;">本次大赛我们收到了远超预期的、众多优秀创想者的数百份申请。经过组委会审慎的评估与艰难的抉择，我们不得不遗憾地告知您，此次您未能进入本次大赛的正式参赛者名单。</p>
              <p style="margin:18px 0 0;color:#4b4036;font-size:15px;line-height:1.85;">我们深知每一份申请背后都承载着对技术的热爱与对创新的追求，虽然此次无法邀请您以选手身份登场，但我们仍然欢迎您在2026年5月22日至24日大赛线下活动期间，亲临天津天开高教科创园核心区现场，与我们一同沉浸在这场科技盛会的氛围中。届时您可以：</p>
              <table role="presentation" width="100%%" cellspacing="0" cellpadding="0" style="border-collapse:separate;border-spacing:0 12px;">
                <tr><td style="padding:16px 18px;border-radius:18px;background:#fffaf0;border:1px solid rgba(36,31,26,0.08);"><strong style="color:#241f1a;">自由观摩：</strong> 近距离观看参赛团队的开发过程与项目展示，感受真实赛场中的创新节奏；</td></tr>
                <tr><td style="padding:16px 18px;border-radius:18px;background:#fffaf0;border:1px solid rgba(36,31,26,0.08);"><strong style="color:#241f1a;">交流学习：</strong> 与来自企业、高校的导师及优秀选手进行面对面沟通，激发灵感、拓展视野；</td></tr>
                <tr><td style="padding:16px 18px;border-radius:18px;background:#fffaf0;border:1px solid rgba(36,31,26,0.08);"><strong style="color:#241f1a;">体验互动：</strong> 参与现场的前沿企业workshop等公开环节。</td></tr>
              </table>
              <p style="margin:20px 0 0;color:#4b4036;font-size:15px;line-height:1.85;">相遇与链接本身，就是创造的开始。如果您希望在此期间到场交流，可直接前往活动现场，现场志愿者将为您提供指引与服务。再次感谢您的关注与支持，也祝愿您在未来的创新道路上持续突破、精彩继续。期待在不久的将来，能与您在更广阔的科创舞台重逢！</p>
              <p style="margin:18px 0 0;color:#4b4036;font-size:15px;line-height:1.75;">如有疑问，请使用微信搜索“15522512264”添加BoHack官方小助手进一步咨询。</p>
              <p style="margin:24px 0 0;color:#241f1a;font-size:15px;line-height:1.7;font-weight:800;">BoHack组委会<br>%s</p>`, escapedName, sentDateText))

	message, err := renderRegistrationCopyEmail(registrationCopyEmailData{
		Subject:         subject,
		Preheader:       "感谢您对2026智能创新黑客松大赛的关注与申请。",
		Eyebrow:         "Application Reply · 申请回复",
		HeroTitle:       "感谢您的申请。",
		HeroSubtitle:    "欢迎您来到现场观摩、交流与体验。",
		BodyHTML:        bodyHTML,
		QRCodeContentID: helperQRCodeContentID,
	}, visitorText(subject, name, sentDateText))
	if err != nil {
		return emailMessage{}, err
	}
	return message, nil
}

func buildMinorAdmissionEmail(name, confirmURL string, sentAt, confirmationDeadline time.Time) (emailMessage, error) {
	name = fallbackName(name)
	escapedName := template.HTMLEscapeString(name)
	escapedConfirmURL := template.HTMLEscapeString(strings.TrimSpace(confirmURL))
	sentAt, confirmationDeadline = resolveConfirmationEmailTimes(sentAt, confirmationDeadline)
	deadlineText := formatConfirmationDeadlineText(confirmationDeadline)
	sentDateText := formatChineseDate(sentAt)
	subject := "祝贺！您的孩子已正式获得2026智能创新黑客松的参赛席位！"
	bodyHTML := template.HTML(fmt.Sprintf(`
              <p style="margin:0 0 18px;color:#241f1a;font-size:18px;line-height:1.7;font-weight:800;">%sBoHacker 的家长：</p>
              <p style="margin:0;color:#4b4036;font-size:15px;line-height:1.85;">您好！经过仔细的评估与筛选，我们非常荣幸地欢迎您的孩子正式加入2026智能创新黑客松！您的孩子在报名材料中所展现出的技术热情、独特思考，以及在过往项目经历中迸发的创造力，让我们坚信，他/她正是我们一直在寻找的“造物者”。在这里，他/她将与志同道合的伙伴一起，在42小时内将创意变为现实，还将获得深度的项目赋能，最终在2026世界智能产业博览会的舞台上，与产业巨头同台，在主流媒体与投资机构的注视下，让创新作品绽放光芒。</p>
              <p style="margin:18px 0 0;color:#4b4036;font-size:15px;line-height:1.85;">我们深知，对于未成年参赛者，家长的了解与支持尤为重要，因此特此向您说明情况，并恳请您仔细阅读以下重要信息，协助完成后续确认流程。</p>
              <div style="margin:30px 0 14px;font-size:13px;letter-spacing:0.12em;text-transform:uppercase;color:#7c6f63;font-weight:800;">一、活动核心信息</div>
              <table role="presentation" width="100%%" cellspacing="0" cellpadding="0" style="border-collapse:separate;border-spacing:0 12px;">
                <tr><td style="padding:16px 18px;border-radius:18px;background:#241f1a;color:#f7f1e5;"><strong style="color:#cff65d;">活动名称</strong><br>2026智能创新黑客松</td></tr>
                <tr><td style="padding:16px 18px;border-radius:18px;background:#241f1a;color:#f7f1e5;"><strong style="color:#cff65d;">42h线下黑客松</strong><br>5月22日——5月24日（天开高教科创园）</td></tr>
                <tr><td style="padding:16px 18px;border-radius:18px;background:#241f1a;color:#f7f1e5;"><strong style="color:#cff65d;">项目赋能</strong><br>5月25日——5月27日（线上）</td></tr>
                <tr><td style="padding:16px 18px;border-radius:18px;background:#241f1a;color:#f7f1e5;"><strong style="color:#cff65d;">展览+路演</strong><br>5月28日——5月31日（国家会展中心）</td></tr>
                <tr><td style="padding:16px 18px;border-radius:18px;background:#241f1a;color:#f7f1e5;"><strong style="color:#cff65d;">报到时间</strong><br>5月22日（周五）16:00</td></tr>
              </table>
              <div style="margin:30px 0 14px;font-size:13px;letter-spacing:0.12em;text-transform:uppercase;color:#7c6f63;font-weight:800;">二、后续步骤与须知</div>
              <table role="presentation" width="100%%" cellspacing="0" cellpadding="0" style="border-collapse:separate;border-spacing:0 12px;">
                <tr><td style="padding:16px 18px;border-radius:18px;background:#fffaf0;border:1px solid rgba(36,31,26,0.08);"><strong style="color:#241f1a;">1. 确认参赛与签署家长知情同意书：</strong>由于您的孩子为未成年人，参赛需获得监护人的书面同意。请您务必于<strong style="color:#241f1a;">%s</strong>点击下方链接确认参赛，认真阅读并上传手写签署后的《2026智能创新黑客松活动风险告知与参与确认书》（见附件），以确认孩子的参赛资格。如因个人安排变动无法如期参与，请回复本邮件告知<strong style="color:#241f1a;">【姓名 + 无法参赛 + 简要原因】</strong>，我们将尽力为您协调或提供协助。逾期未确认将视为自动放弃。<a href="%s" style="display:block;margin-top:16px;padding:16px 22px;border-radius:999px;background:#cff65d;color:#241f1a;text-align:center;text-decoration:none;font-weight:800;">确认参赛并上传确认书 ↗</a></td></tr>
                <tr><td style="padding:16px 18px;border-radius:18px;background:#fffaf0;border:1px solid rgba(36,31,26,0.08);"><strong style="color:#241f1a;">2. 添加小助手微信：</strong> 为确保信息畅通，请使用微信扫描以下二维码或搜索“15522512264”添加BoHack官方小助手，添加时验证信息备注格式为“2026智能创新黑客松-姓名-学校/单位”，后续事项将通过该微信进行通知。<img src="cid:%s" alt="BoHack 小助手微信二维码" width="180" style="display:block;margin-top:14px;width:180px;max-width:100%%;height:auto;border-radius:22px;border:1px solid rgba(36,31,26,0.12);background:#fffdf6;"></td></tr>
                <tr><td style="padding:16px 18px;border-radius:18px;background:#fffaf0;border:1px solid rgba(36,31,26,0.08);"><strong style="color:#241f1a;">3. 活动详情：</strong> 关于活动的详细日程、规则、赛题发布等信息，请持续关注小助手微信消息及BoHack官方微信公众号。</td></tr>
              </table>
              <p style="margin:20px 0 0;color:#4b4036;font-size:15px;line-height:1.85;">九河下梢，海河之畔，创新的潮水正奔腾涌动。我们诚邀您，成为这浪潮中最激越的一脉；我们期待您，带着智慧的星火与不羁的创意而来，用42小时将奇思淬炼为真；我们更将与您一同，将璀璨的成果推上世界瞩目的舞台。天津已准备好见证您的光芒，世界亦是。</p>
              <p style="margin:18px 0 0;color:#4b4036;font-size:15px;line-height:1.75;">如有任何问题，欢迎随时回复本邮件或添加小助手微信咨询。</p>
              <p style="margin:18px 0 0;color:#7c6f63;font-size:13px;line-height:1.7;">附件：%s</p>
              <p style="margin:24px 0 0;color:#241f1a;font-size:15px;line-height:1.7;font-weight:800;">BoHack组委会<br>%s</p>
              <p style="margin:22px 0 0;color:#8b8177;font-size:13px;line-height:1.7;">如果按钮无法打开，请复制链接到浏览器：%s</p>`, escapedName, deadlineText, escapedConfirmURL, helperQRCodeContentID, template.HTMLEscapeString(riskConfirmationFile), sentDateText, escapedConfirmURL))

	message, err := renderRegistrationCopyEmail(registrationCopyEmailData{
		Subject:         subject,
		Preheader:       "您的孩子已正式获得2026智能创新黑客松的参赛席位。",
		Eyebrow:         "Minor Admission · 未成年人录取",
		HeroTitle:       "您的孩子已获得参赛席位。",
		HeroSubtitle:    "请家长协助完成后续确认流程。",
		BodyHTML:        bodyHTML,
		QRCodeContentID: helperQRCodeContentID,
	}, minorAdmissionText(subject, name, confirmURL, deadlineText, sentDateText))
	if err != nil {
		return emailMessage{}, err
	}
	return addCommonRegistrationAssets(message, true)
}

func buildAgreementReminderEmail(name string) (emailMessage, error) {
	name = fallbackName(name)
	escapedName := template.HTMLEscapeString(name)
	subject := "【赛前确认】BoHack2025参赛协议签署与参赛确认通知"
	bodyHTML := template.HTML(fmt.Sprintf(`
              <p style="margin:0 0 18px;color:#241f1a;font-size:18px;line-height:1.7;font-weight:800;">%sBoHacker：</p>
              <p style="margin:0;color:#4b4036;font-size:15px;line-height:1.85;">您好！再次祝贺您获得BoHack2025天津黑客松的参赛席位！我们已开始为12月26日的相聚做最后准备。为确保活动安全、有序进行，并保障每一位参赛者的权益，我们需要请您协助完成两项重要的赛前确认流程，请您理解与支持。</p>
              <div style="margin:30px 0 14px;font-size:13px;letter-spacing:0.12em;text-transform:uppercase;color:#7c6f63;font-weight:800;">第一步：签署参赛协议（至关重要）</div>
              <div style="padding:18px;border-radius:20px;background:#fffaf0;border:1px solid rgba(36,31,26,0.08);">
                请下载并认真阅读附件中的《BoHack2025黑客松活动免责声明》。确认内容无误后，务必于<strong style="color:#241f1a;">【12月23日18点】</strong>前于邮件附件中上传签署后的《BoHack2025 天津黑客松活动免责声明》，以确认您的参赛资格。逾期未签署将视为自动放弃参赛资格。
              </div>
              <div style="margin:30px 0 14px;font-size:13px;letter-spacing:0.12em;text-transform:uppercase;color:#7c6f63;font-weight:800;">第二步：添加小助手，加入官方选手群</div>
              <div style="padding:18px;border-radius:20px;background:#fffaf0;border:1px solid rgba(36,31,26,0.08);">
                所有赛事重要通知、流程更新、组队信息及即时答疑都将在官方微信选手群内发布。若您还未添加小助手，请使用微信扫描下方二维码，添加 【BoHack小助手】。添加时请备注：【姓名-参赛选手】。添加成功后，小助手会邀请您进入官方选手群。
                <img src="cid:%s" alt="BoHack 小助手微信二维码" width="180" style="display:block;margin-top:14px;width:180px;max-width:100%%;height:auto;border-radius:22px;border:1px solid rgba(36,31,26,0.12);background:#fffdf6;">
              </div>
              <p style="margin:20px 0 0;color:#4b4036;font-size:15px;line-height:1.85;">完成以上两步，您的赛前准备就全部就绪了。如有任何疑问，在添加小助手后，可直接在微信上咨询。感谢您的配合！我们期待在天津，与您一同开启这场创新之旅。</p>
              <p style="margin:18px 0 0;color:#7c6f63;font-size:13px;line-height:1.7;">附件：%s</p>`, escapedName, helperQRCodeContentID, template.HTMLEscapeString(riskConfirmationFile)))

	message, err := renderRegistrationCopyEmail(registrationCopyEmailData{
		Subject:         subject,
		Preheader:       "请完成参赛协议签署与小助手添加。",
		Eyebrow:         "Agreement Reminder · 赛前确认",
		HeroTitle:       "请完成赛前确认。",
		HeroSubtitle:    "参赛协议签署与官方选手群信息。",
		BodyHTML:        bodyHTML,
		QRCodeContentID: helperQRCodeContentID,
	}, agreementReminderText(subject, name))
	if err != nil {
		return emailMessage{}, err
	}
	return addCommonRegistrationAssets(message, true)
}

func renderRegistrationCopyEmail(data registrationCopyEmailData, text string) (emailMessage, error) {
	html, err := renderTemplate(registrationCopyEmailTemplate, data)
	if err != nil {
		return emailMessage{}, err
	}
	return emailMessage{
		subject: data.Subject,
		text:    text,
		html:    html,
	}, nil
}

func visitorText(subject, name, sentDateText string) string {
	return fmt.Sprintf(`%s

%sBoHacker：

您好！非常感谢您对2026智能创新黑客松大赛的关注与申请，也诚挚感谢您在申请过程中所付出的时间与思考。我们非常珍视每一位申请者的热情与创意。

本次大赛我们收到了远超预期的、众多优秀创想者的数百份申请。经过组委会审慎的评估与艰难的抉择，我们不得不遗憾地告知您，此次您未能进入本次大赛的正式参赛者名单。

我们深知每一份申请背后都承载着对技术的热爱与对创新的追求，虽然此次无法邀请您以选手身份登场，但我们仍然欢迎您在2026年5月22日至24日大赛线下活动期间，亲临天津天开高教科创园核心区现场，与我们一同沉浸在这场科技盛会的氛围中。届时您可以：

- 自由观摩：近距离观看参赛团队的开发过程与项目展示，感受真实赛场中的创新节奏；
- 交流学习：与来自企业、高校的导师及优秀选手进行面对面沟通，激发灵感、拓展视野；
- 体验互动：参与现场的前沿企业workshop等公开环节。

相遇与链接本身，就是创造的开始。如果您希望在此期间到场交流，可直接前往活动现场，现场志愿者将为您提供指引与服务。再次感谢您的关注与支持，也祝愿您在未来的创新道路上持续突破、精彩继续。期待在不久的将来，能与您在更广阔的科创舞台重逢！

如有疑问，请使用微信搜索“15522512264”添加BoHack官方小助手进一步咨询。

BoHack组委会
%s
BOHACK 2026 · 天津 / 2026.05.22-31 · WIE 2026 OFFICIAL TRACK
`, subject, name, sentDateText)
}

func minorAdmissionText(subject, name, confirmURL, deadlineText, sentDateText string) string {
	return fmt.Sprintf(`%s

%sBoHacker 的家长：

您好！经过仔细的评估与筛选，我们非常荣幸地欢迎您的孩子正式加入2026智能创新黑客松！您的孩子在报名材料中所展现出的技术热情、独特思考，以及在过往项目经历中迸发的创造力，让我们坚信，他/她正是我们一直在寻找的“造物者”。在这里，他/她将与志同道合的伙伴一起，在42小时内将创意变为现实，还将获得深度的项目赋能，最终在2026世界智能产业博览会的舞台上，与产业巨头同台，在主流媒体与投资机构的注视下，让创新作品绽放光芒。

我们深知，对于未成年参赛者，家长的了解与支持尤为重要，因此特此向您说明情况，并恳请您仔细阅读以下重要信息，协助完成后续确认流程。

一、活动核心信息
- 活动名称：2026智能创新黑客松
- 活动时间与地点：
  - 42h线下黑客松：5月22日——5月24日（天开高教科创园）
  - 项目赋能：5月25日——5月27日（线上）
  - 展览+路演：5月28日——5月31日（国家会展中心）
- 报到时间：5月22日（周五）16:00

二、后续步骤与须知
1. 确认参赛与签署家长知情同意书：由于您的孩子为未成年人，参赛需获得监护人的书面同意。请您务必于【%s】点击下方链接确认参赛，认真阅读并上传手写签署后的《2026智能创新黑客松活动风险告知与参与确认书》（见附件），以确认孩子的参赛资格。如因个人安排变动无法如期参与，请回复本邮件告知【姓名 + 无法参赛 + 简要原因】，我们将尽力为您协调或提供协助。逾期未确认将视为自动放弃。
确认链接：%s
2. 添加小助手微信：为确保信息畅通，请使用微信扫描邮件中的二维码或搜索“15522512264”添加BoHack官方小助手，添加时验证信息备注格式为“2026智能创新黑客松-姓名-学校/单位”，后续事项将通过该微信进行通知。
3. 活动详情：关于活动的详细日程、规则、赛题发布等信息，请持续关注小助手微信消息及BoHack官方微信公众号。

九河下梢，海河之畔，创新的潮水正奔腾涌动。我们诚邀您，成为这浪潮中最激越的一脉；我们期待您，带着智慧的星火与不羁的创意而来，用42小时将奇思淬炼为真；我们更将与您一同，将璀璨的成果推上世界瞩目的舞台。天津已准备好见证您的光芒，世界亦是。

如有任何问题，欢迎随时回复本邮件或添加小助手微信咨询。
附件：%s

BoHack组委会
%s
BOHACK 2026 · 天津 / 2026.05.22-31 · WIE 2026 OFFICIAL TRACK
`, subject, name, deadlineText, strings.TrimSpace(confirmURL), riskConfirmationFile, sentDateText)
}

func agreementReminderText(subject, name string) string {
	return fmt.Sprintf(`%s

%sBoHacker：

您好！再次祝贺您获得BoHack2025天津黑客松的参赛席位！我们已开始为12月26日的相聚做最后准备。为确保活动安全、有序进行，并保障每一位参赛者的权益，我们需要请您协助完成两项重要的赛前确认流程，请您理解与支持。

第一步：签署参赛协议（至关重要）
请您按以下要求完成协议签署：
- 请下载并认真阅读附件中的《BoHack2025黑客松活动免责声明》。
- 确认内容无误后，务必于【12月23日18点】前于邮件附件中上传签署后的《BoHack2025 天津黑客松活动免责声明》，以确认您的参赛资格。
- 逾期未签署将视为自动放弃参赛资格。

第二步：添加小助手，加入官方选手群
所有赛事重要通知、流程更新、组队信息及即时答疑都将在官方微信选手群内发布。
- 若您还未添加小助手，请使用微信扫描邮件中的二维码，添加【BoHack小助手】。
- 添加时请备注：【姓名-参赛选手】。
- 添加成功后，小助手会邀请您进入官方选手群。

完成以上两步，您的赛前准备就全部就绪了。如有任何疑问，在添加小助手后，可直接在微信上咨询。感谢您的配合！我们期待在天津，与您一同开启这场创新之旅。
附件：%s
BOHACK 2026 · 天津 / 2026.05.22-31 · WIE 2026 OFFICIAL TRACK
`, subject, name, riskConfirmationFile)
}

func fallbackName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "同学"
	}
	return name
}

func resolveConfirmationEmailTimes(sentAt, confirmationDeadline time.Time) (time.Time, time.Time) {
	if sentAt.IsZero() {
		sentAt = time.Now().UTC()
	}
	if confirmationDeadline.IsZero() {
		confirmationDeadline = sentAt.Add(72 * time.Hour)
	}
	return sentAt.UTC(), confirmationDeadline.UTC()
}

func formatConfirmationDeadlineText(value time.Time) string {
	local := value.In(chinaTimeLocation())
	return fmt.Sprintf("%d月%d日17点前", int(local.Month()), local.Day())
}

func formatChineseDate(value time.Time) string {
	local := value.In(chinaTimeLocation())
	return fmt.Sprintf("%d年%d月%d日", local.Year(), int(local.Month()), local.Day())
}

func chinaTimeLocation() *time.Location {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("CST", 8*60*60)
	}
	return location
}

func addCommonRegistrationAssets(message emailMessage, includeRiskAttachment bool) (emailMessage, error) {
	qrCode, err := emailAssets.ReadFile("assets/" + helperQRCodeFilename)
	if err != nil {
		return emailMessage{}, err
	}
	message.parts = append(message.parts, emailPart{
		filename:    helperQRCodeFilename,
		contentType: "image/png",
		contentID:   helperQRCodeContentID,
		inline:      true,
		data:        qrCode,
	})

	if includeRiskAttachment {
		attachment, err := emailAssets.ReadFile("assets/" + riskConfirmationFile)
		if err != nil {
			return emailMessage{}, err
		}
		message.parts = append(message.parts, emailPart{
			filename:    riskConfirmationFile,
			contentType: "application/pdf",
			data:        attachment,
		})
	}

	return message, nil
}

func renderTemplate(tpl *template.Template, data any) (string, error) {
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return strings.TrimSpace(buf.String()), nil
}

func (m *SMTPMailer) send(to string, message emailMessage) error {
	addr := fmt.Sprintf("%s:%d", m.host, m.port)
	from := m.envelopeFrom()
	recipients := []string{to}
	raw := buildRawMessage(m.from, to, message)

	if m.port == 465 {
		return m.sendImplicitTLS(addr, from, recipients, raw)
	}

	return smtp.SendMail(addr, m.auth(), from, recipients, raw)
}

func buildRawMessage(from, to string, message emailMessage) []byte {
	var buf bytes.Buffer
	writeMessageHeaders(&buf, from, to, message)

	if len(message.parts) == 0 {
		alternativeBoundary := fmt.Sprintf("bohack-alt-%d", time.Now().UnixNano())
		writeHeaderLine(&buf, "Content-Type", mime.FormatMediaType("multipart/alternative", map[string]string{"boundary": alternativeBoundary}))
		writeCRLF(&buf)
		writeAlternativeBody(&buf, alternativeBoundary, message)
		return buf.Bytes()
	}

	mixedBoundary := fmt.Sprintf("bohack-mixed-%d", time.Now().UnixNano())
	relatedBoundary := fmt.Sprintf("bohack-related-%d", time.Now().UnixNano())
	alternativeBoundary := fmt.Sprintf("bohack-alt-%d", time.Now().UnixNano())

	writeHeaderLine(&buf, "Content-Type", mime.FormatMediaType("multipart/mixed", map[string]string{"boundary": mixedBoundary}))
	writeCRLF(&buf)

	writeBoundary(&buf, mixedBoundary)
	writeHeaderLine(&buf, "Content-Type", mime.FormatMediaType("multipart/related", map[string]string{"boundary": relatedBoundary}))
	writeCRLF(&buf)

	writeBoundary(&buf, relatedBoundary)
	writeHeaderLine(&buf, "Content-Type", mime.FormatMediaType("multipart/alternative", map[string]string{"boundary": alternativeBoundary}))
	writeCRLF(&buf)
	writeAlternativeBody(&buf, alternativeBoundary, message)

	for _, part := range message.parts {
		if !part.inline {
			continue
		}
		writeBinaryPart(&buf, relatedBoundary, part)
	}
	writeClosingBoundary(&buf, relatedBoundary)

	for _, part := range message.parts {
		if part.inline {
			continue
		}
		writeBinaryPart(&buf, mixedBoundary, part)
	}
	writeClosingBoundary(&buf, mixedBoundary)

	return buf.Bytes()
}

func writeMessageHeaders(buf *bytes.Buffer, from, to string, message emailMessage) {
	writeHeaderLine(buf, "From", from)
	writeHeaderLine(buf, "To", to)
	writeHeaderLine(buf, "Subject", encodeHeader(message.subject))
	writeHeaderLine(buf, "MIME-Version", "1.0")
}

func writeAlternativeBody(buf *bytes.Buffer, boundary string, message emailMessage) {
	writeBoundary(buf, boundary)
	writeHeaderLine(buf, "Content-Type", "text/plain; charset=UTF-8")
	writeHeaderLine(buf, "Content-Transfer-Encoding", "quoted-printable")
	writeCRLF(buf)
	writeString(buf, encodeQuotedPrintable(message.text))
	writeCRLF(buf)

	writeBoundary(buf, boundary)
	writeHeaderLine(buf, "Content-Type", "text/html; charset=UTF-8")
	writeHeaderLine(buf, "Content-Transfer-Encoding", "quoted-printable")
	writeCRLF(buf)
	writeString(buf, encodeQuotedPrintable(message.html))
	writeCRLF(buf)
	writeClosingBoundary(buf, boundary)
}

func writeBinaryPart(buf *bytes.Buffer, boundary string, part emailPart) {
	writeBoundary(buf, boundary)
	writeHeaderLine(buf, "Content-Type", mime.FormatMediaType(part.contentType, map[string]string{"name": part.filename}))
	writeHeaderLine(buf, "Content-Transfer-Encoding", "base64")
	if part.contentID != "" {
		writeHeaderLine(buf, "Content-ID", "<"+cleanHeader(part.contentID)+">")
	}
	disposition := "attachment"
	if part.inline {
		disposition = "inline"
	}
	writeHeaderLine(buf, "Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": part.filename}))
	writeCRLF(buf)
	writeString(buf, encodeBase64(part.data))
	writeCRLF(buf)
}

func writeBoundary(buf *bytes.Buffer, boundary string) {
	writeString(buf, "--"+boundary)
	writeCRLF(buf)
}

func writeClosingBoundary(buf *bytes.Buffer, boundary string) {
	writeString(buf, "--"+boundary+"--")
	writeCRLF(buf)
}

func writeHeaderLine(buf *bytes.Buffer, key, value string) {
	writeString(buf, key+": "+value)
	writeCRLF(buf)
}

func writeCRLF(buf *bytes.Buffer) {
	writeString(buf, "\r\n")
}

func writeString(buf *bytes.Buffer, value string) {
	_, _ = buf.WriteString(value)
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

func encodeBase64(data []byte) string {
	encoded := base64.StdEncoding.EncodeToString(data)
	if len(encoded) <= 76 {
		return encoded
	}

	var buf strings.Builder
	for len(encoded) > 76 {
		buf.WriteString(encoded[:76])
		buf.WriteString("\r\n")
		encoded = encoded[76:]
	}
	buf.WriteString(encoded)
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
