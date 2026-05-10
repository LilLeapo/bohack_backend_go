package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bohack_backend_go/internal/httpx"
	"bohack_backend_go/internal/mailer"
	"bohack_backend_go/internal/models"
	"bohack_backend_go/internal/repository"
)

type AttendanceHandler struct {
	registrations   *repository.RegistrationRepository
	confirmations   *repository.AttendanceConfirmationRepository
	attachments     *repository.AttachmentRepository
	mailer          mailer.Mailer
	frontendBaseURL string
	ttl             time.Duration
	attachmentDir   string
	maxUploadBytes  int64
}

type attendanceConfirmRequest struct {
	Token  string `json:"token"`
	Status string `json:"status"`
}

func NewAttendanceHandler(
	registrations *repository.RegistrationRepository,
	confirmations *repository.AttendanceConfirmationRepository,
	attachments *repository.AttachmentRepository,
	mailer mailer.Mailer,
	frontendBaseURL string,
	ttl time.Duration,
	attachmentDir string,
	maxUploadBytes int64,
) *AttendanceHandler {
	return &AttendanceHandler{
		registrations:   registrations,
		confirmations:   confirmations,
		attachments:     attachments,
		mailer:          mailer,
		frontendBaseURL: strings.TrimRight(frontendBaseURL, "/"),
		ttl:             ttl,
		attachmentDir:   attachmentDir,
		maxUploadBytes:  maxUploadBytes,
	}
}

func (h *AttendanceHandler) AdminSend(w http.ResponseWriter, r *http.Request) {
	registrationID, ok := readRegistrationID(w, r)
	if !ok {
		return
	}

	registration, err := h.registrations.GetByID(r.Context(), registrationID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, 40411, "registration not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, 50032, "failed to load registration")
		return
	}

	token, tokenHash, err := generateAttendanceToken()
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, 50080, "failed to generate confirmation token")
		return
	}

	sentAt := time.Now().UTC()
	expiresAt := sentAt.Add(h.ttl)
	confirmation, err := h.confirmations.Create(r.Context(), repository.CreateAttendanceConfirmationParams{
		RegistrationID: registration.ID,
		UserID:         registration.UserID,
		TokenHash:      tokenHash,
		SentAt:         sentAt,
		ExpiresAt:      expiresAt,
	})
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, 50081, "failed to create attendance confirmation")
		return
	}

	confirmURL := h.attendanceURL(token, "confirmed")
	declineURL := h.attendanceURL(token, "declined")
	emailDeadlineAt := sentAt.Add(registrationEmailConfirmationDeadlineOffset)
	if err := h.mailer.SendAttendanceConfirmation(
		r.Context(),
		registration.EmailSnapshot,
		registration.RealName,
		registration.EventTitle,
		confirmURL,
		declineURL,
		sentAt,
		emailDeadlineAt,
	); err != nil {
		httpx.Error(w, http.StatusInternalServerError, 50082, "failed to send attendance confirmation email")
		return
	}

	httpx.OK(w, map[string]any{
		"confirmation": confirmation,
		"delivery":     h.mailer.Mode(),
		"confirmUrl":   consoleOnlyURL(h.mailer.Mode(), confirmURL),
		"declineUrl":   consoleOnlyURL(h.mailer.Mode(), declineURL),
	}, "attendance confirmation sent")
}

func (h *AttendanceHandler) Confirm(w http.ResponseWriter, r *http.Request) {
	req := attendanceConfirmRequest{
		Token:  strings.TrimSpace(r.URL.Query().Get("token")),
		Status: strings.TrimSpace(r.URL.Query().Get("status")),
	}
	if r.Method != http.MethodGet {
		var body attendanceConfirmRequest
		if !httpx.DecodeJSON(w, r, &body) {
			return
		}
		req.Token = firstNonEmpty(body.Token, req.Token)
		req.Status = firstNonEmpty(body.Status, req.Status)
	}

	req.Status = strings.TrimSpace(strings.ToLower(req.Status))
	if req.Token == "" {
		httpx.Error(w, http.StatusBadRequest, 42280, "token is required")
		return
	}
	if !isAllowedAttendanceStatus(req.Status) {
		httpx.Error(w, http.StatusBadRequest, 42281, "status must be confirmed or declined")
		return
	}
	if req.Status == "confirmed" {
		httpx.Error(w, http.StatusBadRequest, 42282, "signed confirmation file is required to confirm attendance")
		return
	}

	tokenHash := hashAttendanceToken(req.Token)
	confirmation, err := h.confirmations.GetByTokenHash(r.Context(), tokenHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, 40480, "attendance confirmation not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, 50083, "failed to load attendance confirmation")
		return
	}
	if time.Now().UTC().After(confirmation.ExpiresAt) {
		httpx.Error(w, http.StatusGone, 41080, "attendance confirmation link has expired")
		return
	}

	updated, err := h.confirmations.Respond(r.Context(), tokenHash, req.Status)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, 50084, "failed to update attendance confirmation")
		return
	}

	registration, err := h.registrations.GetByID(r.Context(), updated.RegistrationID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, 40411, "registration not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, 50032, "failed to load registration")
		return
	}
	registration.Attendance = updated

	httpx.OK(w, map[string]any{
		"attendance":   updated,
		"registration": registration,
	}, "attendance confirmed")
}

func (h *AttendanceHandler) ConfirmUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.maxUploadBytes+(64*1024))
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "request body too large") {
			httpx.Error(w, http.StatusRequestEntityTooLarge, 41301, "uploaded file is too large")
			return
		}
		httpx.Error(w, http.StatusBadRequest, 42260, "invalid multipart form data")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	token := strings.TrimSpace(firstNonEmpty(
		r.FormValue("token"),
		r.URL.Query().Get("token"),
	))
	if token == "" {
		httpx.Error(w, http.StatusBadRequest, 42280, "token is required")
		return
	}

	tokenHash := hashAttendanceToken(token)
	confirmation, err := h.confirmations.GetByTokenHash(r.Context(), tokenHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, 40480, "attendance confirmation not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, 50083, "failed to load attendance confirmation")
		return
	}
	if time.Now().UTC().After(confirmation.ExpiresAt) {
		httpx.Error(w, http.StatusGone, 41080, "attendance confirmation link has expired")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, 42262, "file is required")
		return
	}
	defer file.Close()

	if header.Size > 0 && header.Size > h.maxUploadBytes {
		httpx.Error(w, http.StatusRequestEntityTooLarge, 41301, "uploaded file is too large")
		return
	}

	attachment, ok := h.storeSignedConfirmationAttachment(w, r, confirmation, file, header.Filename, header.Size)
	if !ok {
		return
	}

	updated, err := h.confirmations.Respond(r.Context(), tokenHash, "confirmed")
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, 50084, "failed to update attendance confirmation")
		return
	}

	registration, err := h.registrations.GetByID(r.Context(), updated.RegistrationID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, 40411, "registration not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, 50032, "failed to load registration")
		return
	}
	registration.Attendance = updated

	httpx.OK(w, map[string]any{
		"attendance":   updated,
		"attachment":   presentAttachment(attachment),
		"registration": registration,
	}, "attendance confirmed")
}

func (h *AttendanceHandler) storeSignedConfirmationAttachment(
	w http.ResponseWriter,
	r *http.Request,
	confirmation *models.AttendanceConfirmation,
	file multipartFile,
	fileName string,
	headerSize int64,
) (*models.RegistrationAttachment, bool) {
	originalFileName := sanitizeOriginalFileName(fileName)
	ext := strings.ToLower(filepath.Ext(originalFileName))
	detectedType, bodyReader, err := sniffContentType(file)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, 42263, "failed to read uploaded file")
		return nil, false
	}
	if !isAllowedAttachment(ext, detectedType) {
		httpx.Error(w, http.StatusBadRequest, 42264, "unsupported attachment type")
		return nil, false
	}

	storagePath, absPath, err := prepareAttachmentPath(h.attachmentDir, confirmation.RegistrationID, ext)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, 50051, "failed to prepare attachment storage")
		return nil, false
	}

	dst, err := os.Create(absPath)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, 50052, "failed to store attachment")
		return nil, false
	}

	written, copyErr := copyUploadedFile(dst, bodyReader)
	closeErr := dst.Close()
	if copyErr != nil || closeErr != nil {
		cleanupAttachmentFile(absPath)
		httpx.Error(w, http.StatusInternalServerError, 50053, "failed to save attachment")
		return nil, false
	}
	if written <= 0 {
		cleanupAttachmentFile(absPath)
		httpx.Error(w, http.StatusBadRequest, 42265, "uploaded file is empty")
		return nil, false
	}
	if written > h.maxUploadBytes || headerSize > h.maxUploadBytes {
		cleanupAttachmentFile(absPath)
		httpx.Error(w, http.StatusRequestEntityTooLarge, 41301, "uploaded file is too large")
		return nil, false
	}

	attachment, err := h.attachments.Create(r.Context(), repository.CreateAttachmentParams{
		RegistrationID: confirmation.RegistrationID,
		Kind:           "risk_confirmation",
		StoragePath:    storagePath,
		FileName:       originalFileName,
		MimeType:       detectedType,
		FileSize:       written,
	})
	if err != nil {
		cleanupAttachmentFile(absPath)
		httpx.Error(w, http.StatusInternalServerError, 50054, "failed to create attachment record")
		return nil, false
	}

	return attachment, true
}

func (h *AttendanceHandler) attendanceURL(token, status string) string {
	u, err := url.Parse(h.frontendBaseURL + "/attendance-confirm")
	if err != nil {
		return h.frontendBaseURL + "/attendance-confirm?token=" + url.QueryEscape(token) + "&status=" + url.QueryEscape(status)
	}
	q := u.Query()
	q.Set("token", token)
	q.Set("status", status)
	u.RawQuery = q.Encode()
	return u.String()
}

func generateAttendanceToken() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	token := hex.EncodeToString(raw)
	return token, hashAttendanceToken(token), nil
}

func hashAttendanceToken(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}

func isAllowedAttendanceStatus(status string) bool {
	switch status {
	case "confirmed", "declined":
		return true
	default:
		return false
	}
}

func consoleOnlyURL(mode, value string) string {
	if mode == "console" {
		return value
	}
	return ""
}
