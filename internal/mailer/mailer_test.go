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

func TestBuildAttendanceConfirmationEmailIncludesActions(t *testing.T) {
	message, err := buildAttendanceConfirmationEmail(
		"张三",
		"BOHACK 2026",
		"https://bohack.top/attendance-confirm?token=abc&status=confirmed",
		"https://bohack.top/attendance-confirm?token=abc&status=declined",
	)
	if err != nil {
		t.Fatalf("build attendance email: %v", err)
	}

	assertContains(t, message.subject, "参赛时间确认")
	assertContains(t, message.text, "张三")
	assertContains(t, message.text, "天开高教科创园")
	assertContains(t, message.html, "确认参加")
	assertContains(t, message.html, "status=confirmed")
	assertContains(t, message.html, "status=declined")
}

func assertContains(t *testing.T, value, want string) {
	t.Helper()
	if !strings.Contains(value, want) {
		t.Fatalf("expected %q to contain %q", value, want)
	}
}
