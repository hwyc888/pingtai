package handler

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"pingtai/internal/config"
)

func makeChatAttachmentRequest(t *testing.T, files map[string][]byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, content := range files {
		part, err := writer.CreateFormFile("files", name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/agents/chat", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	if err := request.ParseMultipartForm(2 << 20); err != nil {
		t.Fatal(err)
	}
	return request
}

func TestBuildModelUserContentSupportsMultipleTextAndImageFiles(t *testing.T) {
	request := makeChatAttachmentRequest(t, map[string][]byte{
		"first.txt":  []byte("first attachment text"),
		"second.txt": []byte("second attachment text"),
		"diagram.png": {0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A},
	})
	handler := &Handler{Config: &config.Config{MaxUploadBytes: 1024}}
	userContent, storedContent, err := handler.buildModelUserContent(request, "Please review these attachments")
	if err != nil {
		t.Fatal(err)
	}
	parts, ok := userContent.([]map[string]any)
	if !ok {
		t.Fatalf("expected multimodal message parts, got %T", userContent)
	}
	if len(parts) != 2 {
		t.Fatalf("expected text plus one image part, got %d parts", len(parts))
	}
	textPart, ok := parts[0]["text"].(string)
	if !ok {
		t.Fatalf("first part should be text: %#v", parts[0])
	}
	for _, expected := range []string{"first attachment text", "second attachment text", "first.txt", "second.txt"} {
		if !strings.Contains(textPart, expected) {
			t.Errorf("combined text part missing %q: %s", expected, textPart)
		}
	}
	if !strings.Contains(storedContent, "diagram.png") {
		t.Fatalf("stored message should identify image attachment: %q", storedContent)
	}
	imageURL, ok := parts[1]["image_url"].(map[string]any)
	if !ok || !strings.HasPrefix(imageURL["url"].(string), "data:image/png;base64,") {
		t.Fatalf("image should be embedded as a PNG data URL: %#v", parts[1])
	}
}

func TestBuildModelUserContentRejectsUnsupportedFileInBatch(t *testing.T) {
	request := makeChatAttachmentRequest(t, map[string][]byte{
		"allowed.txt": []byte("text"),
		"document.pdf": []byte("not supported in direct model mode"),
	})
	handler := &Handler{Config: &config.Config{MaxUploadBytes: 1024}}
	if _, _, err := handler.buildModelUserContent(request, "question"); err == nil || !strings.Contains(err.Error(), "支持 TXT") {
		t.Fatalf("expected unsupported attachment error, got %v", err)
	}
}

func TestBuildModelUserContentLimitsBatchToTenFiles(t *testing.T) {
	files := make(map[string][]byte)
	for index := 0; index < 11; index++ {
		files[strings.Repeat("f", index+1)+".txt"] = []byte("text")
	}
	request := makeChatAttachmentRequest(t, files)
	handler := &Handler{Config: &config.Config{MaxUploadBytes: 1024}}
	if _, _, err := handler.buildModelUserContent(request, "question"); err == nil || !strings.Contains(err.Error(), "最多上传 10 个") {
		t.Fatalf("expected batch file count error, got %v", err)
	}
}
