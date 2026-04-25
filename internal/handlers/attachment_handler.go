package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"bohack_backend_go/internal/httpx"
	"bohack_backend_go/internal/models"
	"bohack_backend_go/internal/repository"

	"github.com/go-chi/chi/v5"
)

var allowedAttachmentExtensions = map[string]struct{}{
	".7z":   {},
	".doc":  {},
	".docx": {},
	".jpeg": {},
	".jpg":  {},
	".pdf":  {},
	".png":  {},
	".ppt":  {},
	".pptx": {},
	".rar":  {},
	".txt":  {},
	".webp": {},
	".xls":  {},
	".xlsx": {},
	".zip":  {},
}

type AttachmentHandler struct {
	events         *repository.EventRepository
	registrations  *repository.RegistrationRepository
	attachments    *repository.AttachmentRepository
	defaultSlug    string
	attachmentDir  string
	maxUploadBytes int64
}

func NewAttachmentHandler(
	events *repository.EventRepository,
	registrations *repository.RegistrationRepository,
	attachments *repository.AttachmentRepository,
	defaultSlug string,
	attachmentDir string,
	maxUploadBytes int64,
) *AttachmentHandler {
	return &AttachmentHandler{
		events:         events,
		registrations:  registrations,
		attachments:    attachments,
		defaultSlug:    defaultSlug,
		attachmentDir:  attachmentDir,
		maxUploadBytes: maxUploadBytes,
	}
}

func (h *AttachmentHandler) ListMy(w http.ResponseWriter, r *http.Request) {
	user := httpx.CurrentUser(r)
	if user == nil {
		httpx.Error(w, http.StatusUnauthorized, 40115, "unauthorized")
		return
	}

	registration, ok := h.loadCurrentUserRegistration(w, r, user.UID, readEventSlugFromQuery(r, h.defaultSlug))
	if !ok {
		return
	}

	attachments, err := h.attachments.ListByRegistration(r.Context(), registration.ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, 50050, "failed to load attachments")
		return
	}

	httpx.OK(w, presentAttachments(attachments), "OK")
}

func (h *AttachmentHandler) AdminListForRegistration(w http.ResponseWriter, r *http.Request) {
	registrationID, ok := readRegistrationID(w, r)
	if !ok {
		return
	}

	if _, err := h.registrations.GetByID(r.Context(), registrationID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, 40411, "registration not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, 50032, "failed to load registration")
		return
	}

	attachments, err := h.attachments.ListByRegistration(r.Context(), registrationID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, 50050, "failed to load attachments")
		return
	}

	httpx.OK(w, presentAttachments(attachments), "OK")
}

func (h *AttachmentHandler) Upload(w http.ResponseWriter, r *http.Request) {
	user := httpx.CurrentUser(r)
	if user == nil {
		httpx.Error(w, http.StatusUnauthorized, 40115, "unauthorized")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, h.maxUploadBytes+(2*1024*1024))
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

	eventSlug := firstNonEmpty(
		r.FormValue("event_slug"),
		r.FormValue("eventSlug"),
		readEventSlugFromQuery(r, ""),
	)
	if strings.TrimSpace(eventSlug) == "" {
		eventSlug = h.defaultSlug
	}

	registration, ok := h.loadCurrentUserRegistration(w, r, user.UID, eventSlug)
	if !ok {
		return
	}
	if !registrationAllowsAttachmentChanges(registration.Status) {
		httpx.Error(w, http.StatusConflict, 40914, "attachments can no longer be changed for this registration")
		return
	}

	kind := strings.TrimSpace(r.FormValue("kind"))
	if kind == "" {
		kind = "attachment"
	}
	if tooLong(kind, 50) {
		httpx.Error(w, http.StatusBadRequest, 42261, "kind must be 50 characters or fewer")
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

	originalFileName := sanitizeOriginalFileName(header.Filename)
	ext := strings.ToLower(filepath.Ext(originalFileName))
	detectedType, err := sniffContentType(file)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, 42263, "failed to read uploaded file")
		return
	}
	if !isAllowedAttachment(ext, detectedType) {
		httpx.Error(w, http.StatusBadRequest, 42264, "unsupported attachment type")
		return
	}

	storagePath, absPath, err := h.prepareAttachmentPath(registration.ID, ext)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, 50051, "failed to prepare attachment storage")
		return
	}

	dst, err := os.Create(absPath)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, 50052, "failed to store attachment")
		return
	}

	written, copyErr := copyUploadedFile(dst, file)
	closeErr := dst.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(absPath)
		httpx.Error(w, http.StatusInternalServerError, 50053, "failed to save attachment")
		return
	}
	if written <= 0 {
		_ = os.Remove(absPath)
		httpx.Error(w, http.StatusBadRequest, 42265, "uploaded file is empty")
		return
	}
	if written > h.maxUploadBytes {
		_ = os.Remove(absPath)
		httpx.Error(w, http.StatusRequestEntityTooLarge, 41301, "uploaded file is too large")
		return
	}

	attachment, err := h.attachments.Create(r.Context(), repository.CreateAttachmentParams{
		RegistrationID: registration.ID,
		Kind:           kind,
		StoragePath:    storagePath,
		FileName:       originalFileName,
		MimeType:       detectedType,
		FileSize:       written,
	})
	if err != nil {
		_ = os.Remove(absPath)
		httpx.Error(w, http.StatusInternalServerError, 50054, "failed to create attachment record")
		return
	}

	httpx.OK(w, presentAttachment(attachment), "attachment uploaded")
}

func (h *AttachmentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	user := httpx.CurrentUser(r)
	if user == nil {
		httpx.Error(w, http.StatusUnauthorized, 40115, "unauthorized")
		return
	}

	attachmentID, ok := readAttachmentID(w, r)
	if !ok {
		return
	}

	attachment, err := h.attachments.GetByID(r.Context(), attachmentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, 40420, "attachment not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, 50055, "failed to load attachment")
		return
	}
	if attachment.UserID != user.UID {
		httpx.Error(w, http.StatusForbidden, 40320, "forbidden")
		return
	}

	registration, err := h.registrations.GetByID(r.Context(), attachment.RegistrationID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, 40404, "registration not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, 50015, "failed to load registration")
		return
	}
	if !registrationAllowsAttachmentChanges(registration.Status) {
		httpx.Error(w, http.StatusConflict, 40914, "attachments can no longer be changed for this registration")
		return
	}

	if err := h.attachments.Delete(r.Context(), attachment.ID); err != nil {
		httpx.Error(w, http.StatusInternalServerError, 50056, "failed to delete attachment")
		return
	}

	if absPath, err := h.resolveStoragePath(attachment.StoragePath); err == nil {
		if err := os.Remove(absPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			httpx.Error(w, http.StatusInternalServerError, 50057, "failed to remove attachment file")
			return
		}
	}

	httpx.OK(w, map[string]any{"id": attachment.ID}, "attachment deleted")
}

func (h *AttachmentHandler) Download(w http.ResponseWriter, r *http.Request) {
	user := httpx.CurrentUser(r)
	if user == nil {
		httpx.Error(w, http.StatusUnauthorized, 40115, "unauthorized")
		return
	}

	attachmentID, ok := readAttachmentID(w, r)
	if !ok {
		return
	}

	attachment, err := h.attachments.GetByID(r.Context(), attachmentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, 40420, "attachment not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, 50055, "failed to load attachment")
		return
	}

	if attachment.UserID != user.UID && !user.IsAdmin && !strings.EqualFold(user.Role, "admin") {
		httpx.Error(w, http.StatusForbidden, 40320, "forbidden")
		return
	}

	absPath, err := h.resolveStoragePath(attachment.StoragePath)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, 50058, "invalid attachment storage path")
		return
	}

	file, err := os.Open(absPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			httpx.Error(w, http.StatusNotFound, 40421, "attachment file not found")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, 50059, "failed to open attachment")
		return
	}
	defer file.Close()

	if attachment.MimeType != "" {
		w.Header().Set("Content-Type", attachment.MimeType)
	}
	w.Header().Set("Content-Disposition", buildAttachmentDisposition(attachment.FileName))
	http.ServeContent(w, r, attachment.FileName, attachment.CreatedAt, file)
}

func (h *AttachmentHandler) loadCurrentUserRegistration(w http.ResponseWriter, r *http.Request, userID int, eventSlug string) (*models.Registration, bool) {
	event, err := h.events.GetBySlug(r.Context(), strings.TrimSpace(eventSlug))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, 40403, "event not found")
			return nil, false
		}
		httpx.Error(w, http.StatusInternalServerError, 50014, "failed to load event")
		return nil, false
	}

	registration, err := h.registrations.GetByUserAndEvent(r.Context(), userID, event.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpx.Error(w, http.StatusNotFound, 40404, "registration not found")
			return nil, false
		}
		httpx.Error(w, http.StatusInternalServerError, 50015, "failed to load registration")
		return nil, false
	}

	return registration, true
}

func (h *AttachmentHandler) prepareAttachmentPath(registrationID int64, ext string) (string, string, error) {
	dir := filepath.Join(h.attachmentDir, strconv.FormatInt(registrationID, 10))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}

	randomName, err := randomHex(16)
	if err != nil {
		return "", "", err
	}

	fileName := randomName + ext
	absPath := filepath.Join(dir, fileName)
	relPath := filepath.ToSlash(filepath.Join(strconv.FormatInt(registrationID, 10), fileName))
	return relPath, absPath, nil
}

func (h *AttachmentHandler) resolveStoragePath(storagePath string) (string, error) {
	storagePath = filepath.Clean(storagePath)
	if filepath.IsAbs(storagePath) || storagePath == "." || strings.HasPrefix(storagePath, "..") || strings.Contains(storagePath, "../") {
		return "", fmt.Errorf("invalid storage path")
	}

	baseDir, err := filepath.Abs(h.attachmentDir)
	if err != nil {
		return "", err
	}
	absPath := filepath.Join(baseDir, storagePath)
	absPath = filepath.Clean(absPath)
	if absPath != baseDir && !strings.HasPrefix(absPath, baseDir+string(os.PathSeparator)) {
		return "", fmt.Errorf("invalid storage path")
	}
	return absPath, nil
}

func presentAttachments(items []*models.RegistrationAttachment) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, presentAttachment(item))
	}
	return out
}

func presentAttachment(item *models.RegistrationAttachment) map[string]any {
	return map[string]any{
		"id":             item.ID,
		"registrationId": item.RegistrationID,
		"userId":         item.UserID,
		"kind":           item.Kind,
		"fileName":       item.FileName,
		"mimeType":       item.MimeType,
		"fileSize":       item.FileSize,
		"downloadUrl":    fmt.Sprintf("/registration/attachments/%d/download", item.ID),
		"createdAt":      item.CreatedAt,
	}
}

func readAttachmentID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	rawID := strings.TrimSpace(chi.URLParam(r, "attachmentID"))
	attachmentID, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || attachmentID <= 0 {
		httpx.Error(w, http.StatusBadRequest, 42266, "invalid attachment id")
		return 0, false
	}
	return attachmentID, true
}

func sanitizeOriginalFileName(value string) string {
	value = filepath.Base(strings.TrimSpace(value))
	if value == "" || value == "." || value == string(filepath.Separator) {
		return "attachment"
	}
	value = strings.ReplaceAll(value, "\x00", "")
	return value
}

func sniffContentType(file multipartFile) (string, error) {
	var buf [512]byte
	n, err := file.Read(buf[:])
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}

	detected := http.DetectContentType(buf[:n])
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	return detected, nil
}

func isAllowedAttachment(ext, mimeType string) bool {
	ext = strings.ToLower(ext)
	if _, ok := allowedAttachmentExtensions[ext]; !ok {
		return false
	}

	if strings.HasPrefix(mimeType, "image/") {
		switch mimeType {
		case "image/jpeg", "image/png", "image/webp":
			return ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp"
		default:
			return false
		}
	}

	switch ext {
	case ".pdf":
		return mimeType == "application/pdf" || mimeType == "application/octet-stream"
	case ".txt":
		return strings.HasPrefix(mimeType, "text/plain") || mimeType == "application/octet-stream"
	default:
		return true
	}
}

func buildAttachmentDisposition(fileName string) string {
	return "attachment; filename*=UTF-8''" + url.PathEscape(fileName)
}

func randomHex(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func copyUploadedFile(dst *os.File, src io.Reader) (int64, error) {
	return io.Copy(dst, src)
}

type multipartFile interface {
	io.Reader
	io.Seeker
}
