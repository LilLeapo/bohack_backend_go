package httpx

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// AttachmentSigner produces and verifies short-lived signed URLs for
// attachment downloads. The HMAC key is derived from the JWT secret with a
// dedicated label so the signing key is isolated from the auth token key.
//
// Signed URLs let browsers stream a video directly via <video src="..."> or
// <a href="..."> without sending an Authorization header — the capability is
// embedded in the URL itself and expires after TTL.
type AttachmentSigner struct {
	secret []byte
	ttl    time.Duration
}

const attachmentSigningLabel = "bohack/attachment-download/v1"

func NewAttachmentSigner(secret string, ttl time.Duration) *AttachmentSigner {
	derived := deriveAttachmentKey([]byte(secret))
	return &AttachmentSigner{secret: derived, ttl: ttl}
}

func deriveAttachmentKey(rootSecret []byte) []byte {
	h := hmac.New(sha256.New, rootSecret)
	h.Write([]byte(attachmentSigningLabel))
	return h.Sum(nil)
}

// TTL returns the signature lifetime so callers can advertise expiry to
// clients (e.g. for cache hints).
func (s *AttachmentSigner) TTL() time.Duration {
	return s.ttl
}

// Sign returns the exp (unix seconds) and base64url-encoded signature for the
// attachment, valid for the configured TTL.
func (s *AttachmentSigner) Sign(attachmentID int64) (int64, string) {
	exp := time.Now().UTC().Add(s.ttl).Unix()
	sig := base64.RawURLEncoding.EncodeToString(s.signature(attachmentID, exp))
	return exp, sig
}

// SignedQuery returns the "?exp=...&sig=..." query suffix for direct use in a
// URL. The leading "?" is included so the result can be concatenated onto a
// path that has no existing query string.
func (s *AttachmentSigner) SignedQuery(attachmentID int64) string {
	exp, sig := s.Sign(attachmentID)
	v := url.Values{}
	v.Set("exp", strconv.FormatInt(exp, 10))
	v.Set("sig", sig)
	return "?" + v.Encode()
}

// Verify checks the signature for an attachment and confirms the deadline has
// not passed. Comparison is constant-time.
func (s *AttachmentSigner) Verify(attachmentID int64, expRaw, sigRaw string) error {
	if expRaw == "" || sigRaw == "" {
		return errors.New("missing signature")
	}
	exp, err := strconv.ParseInt(expRaw, 10, 64)
	if err != nil {
		return errors.New("invalid exp")
	}
	if time.Now().UTC().Unix() > exp {
		return errors.New("signature expired")
	}
	got, err := base64.RawURLEncoding.DecodeString(sigRaw)
	if err != nil {
		return errors.New("invalid sig encoding")
	}
	expected := s.signature(attachmentID, exp)
	if !hmac.Equal(expected, got) {
		return errors.New("signature mismatch")
	}
	return nil
}

// VerifyRequest pulls exp/sig from the request query and verifies them.
func (s *AttachmentSigner) VerifyRequest(r *http.Request, attachmentID int64) error {
	q := r.URL.Query()
	return s.Verify(attachmentID, q.Get("exp"), q.Get("sig"))
}

func (s *AttachmentSigner) signature(attachmentID, exp int64) []byte {
	h := hmac.New(sha256.New, s.secret)
	fmt.Fprintf(h, "attachment:%d:%d", attachmentID, exp)
	return h.Sum(nil)
}
