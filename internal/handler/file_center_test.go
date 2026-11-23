package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pingtai/internal/config"
	"pingtai/internal/database"
	"pingtai/internal/middleware"
	"pingtai/internal/repository"
	authservice "pingtai/internal/service/auth"
)

func newFileCenterTestHandler(t *testing.T) (*Handler, map[string]string) {
	t.Helper()
	ctx := context.Background()
	db, err := database.Open(ctx, ":memory:")
	if err != nil { t.Fatal(err) }
	t.Cleanup(func() { _ = db.Close() })
	if err := database.Migrate(ctx, db); err != nil { t.Fatal(err) }
	users := repository.NewUserRepository(db)
	cfg := config.Defaults()
	cfg.DatabasePath = filepath.Join(t.TempDir(), "data", "pingtai.db")
	cfg.JWTSecret = strings.Repeat("file-center-test-secret-", 2)
	cfg.MaxUploadBytes = 2 << 20
	cfg.AccessTokenTTL = 30 * time.Minute
	cfg.RefreshTokenTTL = 24 * time.Hour
	appAuth := authservice.NewService(users, cfg.JWTSecret, cfg.AccessTokenTTL, cfg.RefreshTokenTTL)
	identities := make(map[string]string)
	for _, username := range []string{"alice", "bob", "admin"} {
		role := 0
		if username == "admin" { role = 1 }
		if _, err := users.Create(ctx, username, "test-hash", role); err != nil { t.Fatal(err) }
		user, err := users.FindByUsername(ctx, username)
		if err != nil { t.Fatal(err) }
		identity, err := users.EnsureLocalIdentity(ctx, user)
		if err != nil { t.Fatal(err) }
		identities[username] = identity.ID
	}
	return &Handler{Config: &cfg, DB: db, Users: users, Auth: appAuth}, identities
}

func fileCenterToken(t *testing.T, app *Handler, username, userID string) string {
	t.Helper()
	token, err := app.Auth.IssueAccessForIdentity(username, username, userID, "local")
	if err != nil { t.Fatal(err) }
	return token
}

func callFileCenter(t *testing.T, app *Handler, token, method, path string, body io.Reader, contentType, setID string, endpoint http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, body)
	request.Header.Set("Authorization", "Bearer "+token)
	if contentType != "" { request.Header.Set("Content-Type", contentType) }
	if setID != "" { request.SetPathValue("id", setID) }
	response := httptest.NewRecorder()
	middleware.RequireAccess(app.Auth.Tokens(), http.HandlerFunc(endpoint)).ServeHTTP(response, request)
	return response
}

func makeFileUpload(t *testing.T, filename, content, accessMode string) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("access_mode", accessMode); err != nil { t.Fatal(err) }
	part, err := writer.CreateFormFile("files", filename)
	if err != nil { t.Fatal(err) }
	if _, err := part.Write([]byte(content)); err != nil { t.Fatal(err) }
	if err := writer.Close(); err != nil { t.Fatal(err) }
	return &body, writer.FormDataContentType()
}

func decodeFileCenterJSON(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, response.Body.String())
	}
	return payload
}

func TestFileCenterUploadSearchDownloadAndPermissions(t *testing.T) {
	app, identities := newFileCenterTestHandler(t)
	aliceToken := fileCenterToken(t, app, "alice", identities["alice"])
	bobToken := fileCenterToken(t, app, "bob", identities["bob"])
	body, contentType := makeFileUpload(t, "教学工作计划.txt", "本学期教学工作计划包含教学安排和教学质量考核要求。", "private")
	uploaded := callFileCenter(t, app, aliceToken, http.MethodPost, "/files/upload", body, contentType, "", app.UploadFiles)
	if uploaded.Code != http.StatusOK { t.Fatalf("upload status=%d body=%s", uploaded.Code, uploaded.Body.String()) }
	items := decodeFileCenterJSON(t, uploaded)["data"].(map[string]any)["items"].([]any)
	fileID := items[0].(map[string]any)["id"].(string)
	var storageKey string
	if err := app.DB.QueryRowContext(context.Background(), "SELECT storage_key FROM platform_file WHERE id=?", fileID).Scan(&storageKey); err != nil { t.Fatal(err) }
	if _, err := os.Stat(filepath.Join(app.fileStorageDir(), storageKey)); err != nil { t.Fatalf("original file was not persisted: %v", err) }

	search := callFileCenter(t, app, aliceToken, http.MethodGet, "/files/search?q=教学质量", nil, "", "", app.SearchFiles)
	searchItems := decodeFileCenterJSON(t, search)["data"].(map[string]any)["items"].([]any)
	if search.Code != http.StatusOK || len(searchItems) != 1 { t.Fatalf("content search status=%d items=%d body=%s", search.Code, len(searchItems), search.Body.String()) }
	if !strings.Contains(searchItems[0].(map[string]any)["snippet"].(string), "教学质量") { t.Fatalf("search snippet missing match: %#v", searchItems[0]) }

	download := callFileCenter(t, app, aliceToken, http.MethodGet, "/files/"+fileID+"/download", nil, "", fileID, app.DownloadFile)
	if download.Code != http.StatusOK || !strings.Contains(download.Body.String(), "教学质量考核") { t.Fatalf("download status=%d body=%s", download.Code, download.Body.String()) }
	if !strings.Contains(download.Header().Get("Content-Disposition"), "attachment") { t.Fatalf("missing attachment disposition: %q", download.Header().Get("Content-Disposition")) }

	bobSearch := callFileCenter(t, app, bobToken, http.MethodGet, "/files/search?q=教学质量", nil, "", "", app.SearchFiles)
	bobItems := decodeFileCenterJSON(t, bobSearch)["data"].(map[string]any)["items"].([]any)
	if len(bobItems) != 0 { t.Fatalf("private file leaked to another user: %#v", bobItems) }
	bobDownload := callFileCenter(t, app, bobToken, http.MethodGet, "/files/"+fileID+"/download", nil, "", fileID, app.DownloadFile)
	if bobDownload.Code != http.StatusNotFound { t.Fatalf("private file download should be denied, status=%d", bobDownload.Code) }

	sharedBody, sharedType := makeFileUpload(t, "共享通知.txt", "所有师生都可以查看的共享通知。", "shared")
	sharedUpload := callFileCenter(t, app, aliceToken, http.MethodPost, "/files/upload", sharedBody, sharedType, "", app.UploadFiles)
	sharedID := decodeFileCenterJSON(t, sharedUpload)["data"].(map[string]any)["items"].([]any)[0].(map[string]any)["id"].(string)
	bobSharedSearch := callFileCenter(t, app, bobToken, http.MethodGet, "/files/search?q=共享通知", nil, "", "", app.SearchFiles)
	sharedItems := decodeFileCenterJSON(t, bobSharedSearch)["data"].(map[string]any)["items"].([]any)
	if len(sharedItems) != 1 { t.Fatalf("shared file should be visible, got %#v", sharedItems) }
	bobSharedDownload := callFileCenter(t, app, bobToken, http.MethodGet, "/files/"+sharedID+"/download", nil, "", sharedID, app.DownloadFile)
	if bobSharedDownload.Code != http.StatusOK { t.Fatalf("shared download status=%d body=%s", bobSharedDownload.Code, bobSharedDownload.Body.String()) }
}

func TestFileCenterDuplicateDetectionAndDelete(t *testing.T) {
	app, identities := newFileCenterTestHandler(t)
	token := fileCenterToken(t, app, "alice", identities["alice"])
	upload := func() map[string]any {
		body, contentType := makeFileUpload(t, "duplicate.txt", "same bytes for duplicate detection", "private")
		response := callFileCenter(t, app, token, http.MethodPost, "/files/upload", body, contentType, "", app.UploadFiles)
		if response.Code != http.StatusOK { t.Fatalf("upload status=%d body=%s", response.Code, response.Body.String()) }
		return decodeFileCenterJSON(t, response)["data"].(map[string]any)["items"].([]any)[0].(map[string]any)
	}
	first, second := upload(), upload()
	if first["duplicate"] != false || second["duplicate"] != true { t.Fatalf("duplicate detection failed: %#v %#v", first, second) }
	fileID := first["id"].(string)
	response := callFileCenter(t, app, token, http.MethodPost, "/files/"+fileID+"/delete", nil, "", fileID, app.DeleteFile)
	if response.Code != http.StatusOK { t.Fatalf("delete status=%d body=%s", response.Code, response.Body.String()) }
	var count int
	if err := app.DB.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM platform_file WHERE id=?", fileID).Scan(&count); err != nil { t.Fatal(err) }
	if count != 0 { t.Fatal("file metadata should be removed after delete") }
}

func TestFileCenterHelpers(t *testing.T) {
	if got := cleanUploadedFilename("../../folder/资料.txt"); got != "资料.txt" { t.Fatalf("filename not sanitized: %q", got) }
	if got := cleanUploadedFilename("C:\\temp\\secret.pdf"); got != "secret.pdf" { t.Fatalf("Windows path not sanitized: %q", got) }
	if validFileStorageKey("../outside.txt") || validFileStorageKey("folder/file.txt") { t.Fatal("unsafe storage key accepted") }
	if !fileAccessAllowed("alice-id", "private", "alice-id", false) ||
		!fileAccessAllowed("alice-id", "shared", "bob-id", false) ||
		fileAccessAllowed("alice-id", "private", "bob-id", false) ||
		!fileAccessAllowed("alice-id", "private", "admin-id", true) { t.Fatal("unexpected file access decision") }
	if snippet := fileSearchSnippet("前面内容。这里包含教学质量考核，后面还有内容。", "教学质量", 60); !strings.Contains(snippet, "教学质量") { t.Fatalf("snippet missing match: %q", snippet) }
}
