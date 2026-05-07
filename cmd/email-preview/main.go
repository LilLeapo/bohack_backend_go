package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"bohack_backend_go/internal/mailer"
)

func main() {
	emailType := flag.String("type", "admission", "email type: admission, visitor, minor_admission, agreement_reminder")
	name := flag.String("name", "张三", "recipient display name")
	confirmURL := flag.String("confirm-url", "https://bohack.top/attendance-confirm?token=preview&status=confirmed", "admission confirmation URL")
	outputDir := flag.String("out", "tmp/email-previews", "output directory")
	flag.Parse()

	kind, ok := mailer.ParseRegistrationEmailKind(*emailType)
	if !ok {
		log.Fatalf("unsupported email type %q", *emailType)
	}

	preview, err := mailer.BuildRegistrationEmailPreview(mailer.RegistrationEmailParams{
		Kind:       kind,
		Name:       *name,
		ConfirmURL: *confirmURL,
	})
	if err != nil {
		log.Fatalf("build email preview: %v", err)
	}

	if err := os.MkdirAll(*outputDir, 0o755); err != nil {
		log.Fatalf("create output dir: %v", err)
	}

	html := strings.ReplaceAll(preview.HTML, "cid:bohack-helper-qr", "xiaozhushou-wxqr.png")
	htmlPath := filepath.Join(*outputDir, string(kind)+".html")
	if err := os.WriteFile(htmlPath, []byte(html), 0o644); err != nil {
		log.Fatalf("write html preview: %v", err)
	}

	qrSource := filepath.Join("internal", "mailer", "assets", "xiaozhushou-wxqr.png")
	qrData, err := os.ReadFile(qrSource)
	if err != nil {
		log.Fatalf("read qr image: %v", err)
	}
	if err := os.WriteFile(filepath.Join(*outputDir, "xiaozhushou-wxqr.png"), qrData, 0o644); err != nil {
		log.Fatalf("write qr image: %v", err)
	}

	fmt.Printf("subject: %s\n", preview.Subject)
	fmt.Printf("html: %s\n", htmlPath)
	if len(preview.Attachments) > 0 {
		fmt.Printf("attachments: %s\n", strings.Join(preview.Attachments, ", "))
	}
}
