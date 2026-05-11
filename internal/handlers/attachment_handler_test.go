package handlers

import "testing"

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

func TestIsAllowedAttachmentRejectsUnsupportedVideoExtension(t *testing.T) {
	if isAllowedAttachment(".exe", "video/mp4") {
		t.Fatal("isAllowedAttachment accepted unsupported extension")
	}
}
