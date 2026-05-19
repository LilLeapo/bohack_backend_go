package httpx

import (
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAttachmentSignerSignAndVerifyRoundTrip(t *testing.T) {
	signer := NewAttachmentSigner("super-secret", 5*time.Minute)

	query := signer.SignedQuery(42)
	if !strings.HasPrefix(query, "?") || !strings.Contains(query, "exp=") || !strings.Contains(query, "sig=") {
		t.Fatalf("unexpected query format: %q", query)
	}

	req := httptest.NewRequest("GET", "/attachments/42/download"+query, nil)
	if err := signer.VerifyRequest(req, 42); err != nil {
		t.Fatalf("verify round trip failed: %v", err)
	}
}

func TestAttachmentSignerRejectsTamperedID(t *testing.T) {
	signer := NewAttachmentSigner("super-secret", 5*time.Minute)
	query := signer.SignedQuery(42)
	req := httptest.NewRequest("GET", "/attachments/43/download"+query, nil)

	if err := signer.VerifyRequest(req, 43); err == nil {
		t.Fatal("expected verification to fail when attachmentID is swapped, got nil error")
	}
}

func TestAttachmentSignerRejectsExpired(t *testing.T) {
	signer := NewAttachmentSigner("super-secret", -1*time.Second)
	exp, sig := signer.Sign(99)
	if err := signer.Verify(99, strconv.FormatInt(exp, 10), sig); err == nil {
		t.Fatal("expected expired signature to be rejected")
	}
}

func TestAttachmentSignerRejectsWrongSecret(t *testing.T) {
	signerA := NewAttachmentSigner("secret-a", time.Hour)
	signerB := NewAttachmentSigner("secret-b", time.Hour)

	exp, sig := signerA.Sign(7)
	if err := signerB.Verify(7, strconv.FormatInt(exp, 10), sig); err == nil {
		t.Fatal("expected signature signed by different secret to be rejected")
	}
}

func TestAttachmentSignerRejectsMissingParams(t *testing.T) {
	signer := NewAttachmentSigner("secret", time.Hour)
	req := httptest.NewRequest("GET", "/attachments/1/download", nil)
	if err := signer.VerifyRequest(req, 1); err == nil {
		t.Fatal("expected missing signature to be rejected")
	}
}

func TestAttachmentSignerRejectsTamperedSig(t *testing.T) {
	signer := NewAttachmentSigner("secret", time.Hour)
	exp, sig := signer.Sign(11)
	tampered := sig[:len(sig)-1] + "A"
	if tampered == sig {
		tampered = sig[:len(sig)-1] + "B"
	}
	if err := signer.Verify(11, strconv.FormatInt(exp, 10), tampered); err == nil {
		t.Fatal("expected tampered signature to be rejected")
	}
}
