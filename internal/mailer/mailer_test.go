package mailer

import (
	"strings"
	"testing"
)

func TestBuildVerificationCodeEmailUsesRegistrationCopy(t *testing.T) {
	message, err := buildVerificationCodeEmail("123456", "register")
	if err != nil {
		t.Fatalf("build verification email: %v", err)
	}

	assertContains(t, message.subject, "注册验证码")
	assertContains(t, message.text, "123456")
	assertContains(t, message.html, "创建账号")
	assertContains(t, message.html, "Registration Code")
}

func TestBuildAttendanceConfirmationEmailUsesAdmissionCopy(t *testing.T) {
	message, err := buildAttendanceConfirmationEmail(
		"张三",
		"BOHACK 2026",
		"https://bohack.top/attendance-confirm?token=abc&status=confirmed",
		"https://bohack.top/attendance-confirm?token=abc&status=declined",
	)
	if err != nil {
		t.Fatalf("build attendance email: %v", err)
	}

	assertContains(t, message.subject, "祝贺！您已正式获得2026智能创新黑客松的参赛席位！")
	assertContains(t, message.text, "张三BoHacker")
	assertContains(t, message.text, "确认链接：https://bohack.top/attendance-confirm?token=abc&status=confirmed")
	assertContains(t, message.text, "2026智能创新黑客松活动风险告知与参与确认书")
	assertContains(t, message.text, "天开高教科创园")
	assertContains(t, message.html, "国家会展中心")
	assertContains(t, message.html, "确认参赛并上传确认书")
	assertContains(t, message.html, "小助手微信二维码")
	assertContains(t, message.html, "cid:bohack-helper-qr")
	assertContains(t, message.html, "status=confirmed")
	assertNotContains(t, message.html, "status=declined")
	if len(message.parts) != 2 {
		t.Fatalf("parts len = %d, want 2", len(message.parts))
	}
	if !message.parts[0].inline || message.parts[0].contentID != helperQRCodeContentID {
		t.Fatalf("first part = %#v, want inline helper qr", message.parts[0])
	}
	assertContains(t, message.parts[1].filename, "2026智能创新黑客松活动风险告知与参与确认书.pdf")
}

func TestBuildRegistrationEmailSupportsCallableKinds(t *testing.T) {
	cases := []struct {
		kind      RegistrationEmailKind
		want      string
		wantParts int
	}{
		{RegistrationEmailVisitor, "体验者", 1},
		{RegistrationEmailMinorAdmission, "家长", 2},
		{RegistrationEmailAgreementReminder, "赛前确认", 2},
	}

	for _, tc := range cases {
		message, err := buildRegistrationEmail(RegistrationEmailParams{Kind: tc.kind, Name: "李四"})
		if err != nil {
			t.Fatalf("build registration email kind=%s: %v", tc.kind, err)
		}
		assertContains(t, message.html, tc.want)
		assertContains(t, message.html, "cid:bohack-helper-qr")
		if len(message.parts) != tc.wantParts {
			t.Fatalf("kind=%s parts len = %d, want %d", tc.kind, len(message.parts), tc.wantParts)
		}
	}
}

func assertContains(t *testing.T, value, want string) {
	t.Helper()
	if !strings.Contains(value, want) {
		t.Fatalf("expected %q to contain %q", value, want)
	}
}

func assertNotContains(t *testing.T, value, unwanted string) {
	t.Helper()
	if strings.Contains(value, unwanted) {
		t.Fatalf("expected %q not to contain %q", value, unwanted)
	}
}
