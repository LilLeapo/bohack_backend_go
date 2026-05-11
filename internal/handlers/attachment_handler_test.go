package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"bohack_backend_go/internal/auth"
	"bohack_backend_go/internal/config"
	"bohack_backend_go/internal/db"
	"bohack_backend_go/internal/httpx"
	"bohack_backend_go/internal/models"
	"bohack_backend_go/internal/repository"
)

func TestIsAllowedAttachmentSupportsRecruitmentVideoTypes(t *testing.T) {
	cases := []struct {
		name     string
		ext      string
		mimeType string
	}{
		{name: "mp4", ext: ".mp4", mimeType: "video/mp4"},
		{name: "m4v", ext: ".m4v", mimeType: "video/mp4"},
		{name: "mov", ext: ".mov", mimeType: "video/quicktime"},
		{name: "webm", ext: ".webm", mimeType: "video/webm"},
		{name: "sniffer fallback", ext: ".mp4", mimeType: "application/octet-stream"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !isAllowedAttachment(tc.ext, tc.mimeType) {
				t.Fatalf("isAllowedAttachment(%q, %q) = false, want true", tc.ext, tc.mimeType)
			}
		})
	}
}

func TestIsAllowedAttachmentSupportsPitchDeckTypes(t *testing.T) {
	cases := []struct {
		name     string
		ext      string
		mimeType string
	}{
		{name: "pptx as zip", ext: ".pptx", mimeType: "application/zip"},
		{name: "pptx as octet stream", ext: ".pptx", mimeType: "application/octet-stream"},
		{name: "keynote", ext: ".key", mimeType: "application/zip"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !isAllowedAttachment(tc.ext, tc.mimeType) {
				t.Fatalf("isAllowedAttachment(%q, %q) = false, want true", tc.ext, tc.mimeType)
			}
		})
	}
}

func TestIsAllowedAttachmentRejectsUnsupportedVideoExtension(t *testing.T) {
	if isAllowedAttachment(".exe", "video/mp4") {
		t.Fatal("isAllowedAttachment accepted unsupported extension")
	}
}

func TestUploadDogfoodMatureProjectRecruitmentAttachments(t *testing.T) {
	handler, token, attachmentRepo, registrationID, attachmentDir := newTestAttachmentUploadHandler(t)

	cases := []struct {
		name     string
		kind     string
		fileName string
		body     []byte
	}{
		{
			name:     "pitch deck pptx",
			kind:     "roadshow_pitch_deck",
			fileName: "deck.pptx",
			body:     []byte("PK\x03\x04\x14\x00\x06\x00pptx content"),
		},
		{
			name:     "pitch deck pdf",
			kind:     "roadshow_pitch_deck",
			fileName: "deck.pdf",
			body:     []byte("%PDF-1.7\npitch deck pdf content\n%%EOF"),
		},
		{
			name:     "pitch deck keynote",
			kind:     "roadshow_pitch_deck",
			fileName: "deck.key",
			body:     []byte("PK\x03\x04\x14\x00\x00\x00keynote content"),
		},
		{
			name:     "pitch video mp4",
			kind:     "roadshow_pitch_video",
			fileName: "demo.mp4",
			body:     []byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isomdemo"),
		},
		{
			name:     "demo video mp4",
			kind:     "roadshow_demo_video",
			fileName: "demo-alias.mp4",
			body:     []byte("\x00\x00\x00\x18ftypisom\x00\x00\x00\x00isommp42demo"),
		},
		{
			name:     "pitch video mov",
			kind:     "roadshow_pitch_video",
			fileName: "demo.mov",
			body:     []byte("\x00\x00\x00\x14ftypqt  \x00\x00\x00\x00qt  demo"),
		},
		{
			name:     "pitch video webm",
			kind:     "roadshow_pitch_video",
			fileName: "demo.webm",
			body:     []byte("\x1a\x45\xdf\xa3webm demo content"),
		},
		{
			name:     "hardware spec pdf",
			kind:     "roadshow_hardware_spec",
			fileName: "hardware-spec.pdf",
			body:     []byte("%PDF-1.7\nhardware spec\n%%EOF"),
		},
		{
			name:     "awards file",
			kind:     "roadshow_awards",
			fileName: "awards.pdf",
			body:     []byte("%PDF-1.7\nawards\n%%EOF"),
		},
		{
			name:     "media reports file",
			kind:     "roadshow_media_reports",
			fileName: "media-reports.pdf",
			body:     []byte("%PDF-1.7\nmedia reports\n%%EOF"),
		},
		{
			name:     "user feedback file",
			kind:     "roadshow_user_feedback",
			fileName: "user-feedback.xlsx",
			body:     []byte("PK\x03\x04\x14\x00\x06\x00xlsx content"),
		},
		{
			name:     "other materials zip",
			kind:     "roadshow_other_materials",
			fileName: "supporting-files.zip",
			body:     []byte("PK\x03\x04\x14\x00\x00\x00supporting files"),
		},
	}

	wantKindCounts := map[string]int{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := performAttachmentUploadRequest(t, handler, token, tc.kind, tc.fileName, tc.body)
			if resp.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d, body = %s", resp.Code, http.StatusOK, resp.Body.String())
			}
		})
		wantKindCounts[tc.kind]++
	}

	attachments, err := attachmentRepo.ListByRegistration(context.Background(), registrationID)
	if err != nil {
		t.Fatalf("list attachments: %v", err)
	}
	if len(attachments) != len(cases) {
		t.Fatalf("attachments len = %d, want %d", len(attachments), len(cases))
	}

	gotKindCounts := map[string]int{}
	for _, item := range attachments {
		gotKindCounts[item.Kind]++
		if item.FileSize <= 0 {
			t.Fatalf("attachment %s file size = %d, want > 0", item.Kind, item.FileSize)
		}
		if _, err := os.Stat(filepath.Join(attachmentDir, item.StoragePath)); err != nil {
			t.Fatalf("stored file for %s does not exist: %v", item.Kind, err)
		}
	}
	for kind, want := range wantKindCounts {
		if got := gotKindCounts[kind]; got != want {
			t.Fatalf("attachment kind %q count = %d, want %d", kind, got, want)
		}
	}
}

func TestUploadDogfoodLargeRecruitmentVideo(t *testing.T) {
	handler, token, attachmentRepo, registrationID, attachmentDir := newTestAttachmentUploadHandler(t)

	body := makeMP4Body(25 * 1024 * 1024)
	resp := performAttachmentUploadRequest(t, handler, token, "roadshow_pitch_video", "large-demo.mp4", body)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", resp.Code, http.StatusOK, resp.Body.String())
	}

	attachments, err := attachmentRepo.ListByRegistration(context.Background(), registrationID)
	if err != nil {
		t.Fatalf("list attachments: %v", err)
	}
	if len(attachments) != 1 {
		t.Fatalf("attachments len = %d, want 1", len(attachments))
	}

	item := attachments[0]
	if item.Kind != "roadshow_pitch_video" {
		t.Fatalf("attachment kind = %q, want roadshow_pitch_video", item.Kind)
	}
	if item.FileSize != int64(len(body)) {
		t.Fatalf("attachment file size = %d, want %d", item.FileSize, len(body))
	}
	stat, err := os.Stat(filepath.Join(attachmentDir, item.StoragePath))
	if err != nil {
		t.Fatalf("stored file does not exist: %v", err)
	}
	if stat.Size() != int64(len(body)) {
		t.Fatalf("stored file size = %d, want %d", stat.Size(), len(body))
	}
}

func TestUploadDogfoodRejectsFileOverConfiguredLimit(t *testing.T) {
	handler, token, _, _, _ := newTestAttachmentUploadHandlerWithMaxUpload(t, 1*1024*1024)

	body := makeMP4Body(2 * 1024 * 1024)
	resp := performAttachmentUploadRequest(t, handler, token, "roadshow_pitch_video", "too-large-demo.mp4", body)
	if resp.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d, body = %s", resp.Code, http.StatusRequestEntityTooLarge, resp.Body.String())
	}
}

func TestDogfoodRoadshowRegistrationThenUploadAttachments(t *testing.T) {
	registrationHandler, attachmentHandler, token, registrationRepo, attachmentRepo, userID, eventID, attachmentDir := newTestRoadshowFlowHandlers(t)

	resp := performRegistrationJSONRequest(t, registrationHandler, token, roadshowRegistrationPayload())
	if resp.Code != http.StatusOK {
		t.Fatalf("registration status = %d, want %d, body = %s", resp.Code, http.StatusOK, resp.Body.String())
	}

	registration, err := registrationRepo.GetByUserAndEvent(context.Background(), userID, eventID)
	if err != nil {
		t.Fatalf("load created registration: %v", err)
	}
	var extra map[string]any
	if err := json.Unmarshal(registration.Extra, &extra); err != nil {
		t.Fatalf("unmarshal registration extra: %v", err)
	}
	if extra["formType"] != "roadshow" {
		t.Fatalf("extra.formType = %v, want roadshow", extra["formType"])
	}
	questionnaires, ok := extra["questionnaires"].(map[string]any)
	if !ok {
		t.Fatalf("extra.questionnaires missing or invalid: %#v", extra["questionnaires"])
	}
	if _, ok := questionnaires["roadshow"].(map[string]any); !ok {
		t.Fatalf("extra.questionnaires.roadshow missing or invalid: %#v", questionnaires["roadshow"])
	}

	uploads := []struct {
		kind     string
		fileName string
		body     []byte
	}{
		{kind: "roadshow_pitch_deck", fileName: "roadshow-deck.pptx", body: []byte("PK\x03\x04\x14\x00\x06\x00pptx content")},
		{kind: "roadshow_demo_video", fileName: "roadshow-demo.mp4", body: makeMP4Body(2 * 1024 * 1024)},
	}
	for _, upload := range uploads {
		resp := performAttachmentUploadRequest(t, attachmentHandler, token, upload.kind, upload.fileName, upload.body)
		if resp.Code != http.StatusOK {
			t.Fatalf("upload %s status = %d, want %d, body = %s", upload.kind, resp.Code, http.StatusOK, resp.Body.String())
		}
	}

	attachments, err := attachmentRepo.ListByRegistration(context.Background(), registration.ID)
	if err != nil {
		t.Fatalf("list attachments: %v", err)
	}
	if len(attachments) != len(uploads) {
		t.Fatalf("attachments len = %d, want %d", len(attachments), len(uploads))
	}
	for _, item := range attachments {
		if _, err := os.Stat(filepath.Join(attachmentDir, item.StoragePath)); err != nil {
			t.Fatalf("stored file for %s does not exist: %v", item.Kind, err)
		}
	}
}

func performRegistrationJSONRequest(t *testing.T, handler http.Handler, token string, payload map[string]any) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal registration payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/registration", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	return resp
}

func performAttachmentUploadRequest(t *testing.T, handler http.Handler, token, kind, fileName string, fileBody []byte) *httptest.ResponseRecorder {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("kind", kind); err != nil {
		t.Fatalf("write kind field: %v", err)
	}
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		t.Fatalf("create file field: %v", err)
	}
	if _, err := part.Write(fileBody); err != nil {
		t.Fatalf("write file field: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/registration/attachments", &body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	return resp
}

func makeMP4Body(size int) []byte {
	body := make([]byte, size)
	copy(body, []byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom"))
	return body
}

func roadshowRegistrationPayload() map[string]any {
	questionnaire := map[string]any{
		"projectName":       "青椒排版agent",
		"projectTagline":    "用AI协助用户处理多种Word文档格式",
		"projectIntro":      "面向成熟项目路演的AI排版工具。",
		"productForm":       []any{"software"},
		"productFormText":   "软件产品",
		"roadshowTeamName":  "青椒科技",
		"leaderName":        "曾志博",
		"leaderContact":     "13831879096",
		"teamMembers":       []any{"成员A", "成员B"},
		"projectStage":      []any{"demo", "users"},
		"projectStageText":  "已完成可演示产品，已有真实用户测试",
		"achievements":      "已线上试点。",
		"nextPlan":          "继续完善商业化落地。",
		"codeRepo":          "https://example.com/repo",
		"hardwareDesc":      "",
		"awardsText":        "暂无",
		"pastCompetitions":  "暂无",
		"mediaReportsText":  "暂无",
		"userFeedbackText":  "用户反馈良好",
		"cooperation":       "寻求渠道与投资合作",
		"isSoftwareProject": true,
		"isHardwareProject": false,
		"title":             "路演项目招募",
		"formType":          "roadshow",
		"formLabel":         "路演项目招募",
	}

	return map[string]any{
		"realName":       "曾志博",
		"phone":          "13831879096",
		"school":         "青椒科技",
		"bio":            "面向成熟项目路演的AI排版工具。",
		"teamName":       "青椒科技",
		"rolePreference": "软件产品",
		"source":         "bohack-questionnaire-roadshow",
		"note":           "用AI协助用户处理多种Word文档格式",
		"extra": map[string]any{
			"questionnaires": map[string]any{
				"roadshow": questionnaire,
			},
			"submittedQuestionnaires": []any{"roadshow"},
			"questionnaire":           questionnaire,
			"formType":                "roadshow",
			"formLabel":               "路演项目招募",
			"projectName":             "青椒排版agent",
			"projectTagline":          "用AI协助用户处理多种Word文档格式",
			"productForm":             "软件产品",
			"projectStage":            "已完成可演示产品，已有真实用户测试",
			"isSoftwareProject":       true,
			"isHardwareProject":       false,
		},
	}
}

func newTestAttachmentUploadHandler(t *testing.T) (http.Handler, string, *repository.AttachmentRepository, int64, string) {
	t.Helper()

	return newTestAttachmentUploadHandlerWithMaxUpload(t, 200*1024*1024)
}

func newTestRoadshowFlowHandlers(t *testing.T) (
	http.Handler,
	http.Handler,
	string,
	*repository.RegistrationRepository,
	*repository.AttachmentRepository,
	int,
	int64,
	string,
) {
	t.Helper()

	cfg := config.Config{
		DBDriver:          "sqlite",
		DatabaseURL:       filepath.Join(t.TempDir(), "test.db"),
		DefaultEventSlug:  "test-event",
		DefaultEventTitle: "Test Event",
	}

	ctx := context.Background()
	gormDB, err := db.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(gormDB); err != nil {
			t.Fatalf("close test database: %v", err)
		}
	})

	if err := db.EnsureSchema(ctx, gormDB, cfg); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}

	userRepo := repository.NewUserRepository(gormDB)
	eventRepo := repository.NewEventRepository(gormDB)
	registrationRepo := repository.NewRegistrationRepository(gormDB)
	attachmentRepo := repository.NewAttachmentRepository(gormDB)

	now := time.Now().UTC()
	user := models.User{
		Username:     "roadshow-user",
		Email:        "roadshow-user@example.com",
		PasswordHash: "hash",
		Role:         "visitor",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := userRepo.Create(ctx, &user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	event, err := eventRepo.GetBySlug(ctx, cfg.DefaultEventSlug)
	if err != nil {
		t.Fatalf("load event: %v", err)
	}

	attachmentDir := t.TempDir()
	registrationHandler := NewRegistrationHandler(eventRepo, registrationRepo, cfg.DefaultEventSlug)
	attachmentHandler := NewAttachmentHandler(
		eventRepo,
		registrationRepo,
		attachmentRepo,
		cfg.DefaultEventSlug,
		attachmentDir,
		200*1024*1024,
	)

	tokenManager := auth.NewTokenManager("test-secret", time.Hour)
	token, _, err := tokenManager.CreateAccessToken(user.UID)
	if err != nil {
		t.Fatalf("create access token: %v", err)
	}

	authMiddleware := httpx.AuthMiddleware(tokenManager, userRepo)
	return authMiddleware(http.HandlerFunc(registrationHandler.Create)),
		authMiddleware(http.HandlerFunc(attachmentHandler.Upload)),
		token,
		registrationRepo,
		attachmentRepo,
		user.UID,
		event.ID,
		attachmentDir
}

func newTestAttachmentUploadHandlerWithMaxUpload(t *testing.T, maxUploadBytes int64) (http.Handler, string, *repository.AttachmentRepository, int64, string) {
	t.Helper()

	cfg := config.Config{
		DBDriver:          "sqlite",
		DatabaseURL:       filepath.Join(t.TempDir(), "test.db"),
		DefaultEventSlug:  "test-event",
		DefaultEventTitle: "Test Event",
	}

	ctx := context.Background()
	gormDB, err := db.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(gormDB); err != nil {
			t.Fatalf("close test database: %v", err)
		}
	})

	if err := db.EnsureSchema(ctx, gormDB, cfg); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}

	userRepo := repository.NewUserRepository(gormDB)
	eventRepo := repository.NewEventRepository(gormDB)
	registrationRepo := repository.NewRegistrationRepository(gormDB)
	attachmentRepo := repository.NewAttachmentRepository(gormDB)

	now := time.Now().UTC()
	user := models.User{
		Username:     "attachment-user",
		Email:        "attachment-user@example.com",
		PasswordHash: "hash",
		Role:         "visitor",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := userRepo.Create(ctx, &user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	event, err := eventRepo.GetBySlug(ctx, cfg.DefaultEventSlug)
	if err != nil {
		t.Fatalf("load event: %v", err)
	}
	registration, err := registrationRepo.Create(ctx, repository.CreateRegistrationParams{
		EventID:       event.ID,
		UserID:        user.UID,
		Status:        "submitted",
		RealName:      "Alice",
		Phone:         "13800138000",
		EmailSnapshot: user.Email,
	})
	if err != nil {
		t.Fatalf("create registration: %v", err)
	}

	attachmentDir := t.TempDir()
	attachmentHandler := NewAttachmentHandler(
		eventRepo,
		registrationRepo,
		attachmentRepo,
		cfg.DefaultEventSlug,
		attachmentDir,
		maxUploadBytes,
	)

	tokenManager := auth.NewTokenManager("test-secret", time.Hour)
	token, _, err := tokenManager.CreateAccessToken(user.UID)
	if err != nil {
		t.Fatalf("create access token: %v", err)
	}

	handler := httpx.AuthMiddleware(tokenManager, userRepo)(http.HandlerFunc(attachmentHandler.Upload))
	return handler, token, attachmentRepo, registration.ID, attachmentDir
}
