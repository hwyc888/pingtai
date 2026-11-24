package handler

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"pingtai/internal/middleware"
)

func createTestKnowledgeBase(t *testing.T, app *Handler, baseID, owner, ownerID, accessMode string) {
	t.Helper()
	ctx := context.Background()
	if _, err := app.DB.ExecContext(ctx,
		"INSERT INTO ai_knowledge_base (id,name,description,enabled,created_by,created_at,updated_at) VALUES (?,?, '',1,?,'now','now')",
		baseID, baseID, owner,
	); err != nil { t.Fatal(err) }
	if _, err := app.DB.ExecContext(ctx,
		"INSERT INTO ai_knowledge_base_policy (base_id,owner_username,access_mode) VALUES (?,?,?)",
		baseID, owner, accessMode,
	); err != nil { t.Fatal(err) }
	if _, err := app.DB.ExecContext(ctx,
		"INSERT INTO ai_knowledge_base_identity (base_id,owner_user_id,access_mode) VALUES (?,?,?)",
		baseID, ownerID, accessMode,
	); err != nil { t.Fatal(err) }
}

func makeKnowledgeDocumentUpload(t *testing.T, baseID, filename, content string) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("base_id", baseID); err != nil { t.Fatal(err) }
	part, err := writer.CreateFormFile("files", filename)
	if err != nil { t.Fatal(err) }
	if _, err := part.Write([]byte(content)); err != nil { t.Fatal(err) }
	if err := writer.Close(); err != nil { t.Fatal(err) }
	return &body, writer.FormDataContentType()
}

func TestKnowledgeUploadAppearsInFileCenterAndCanBeUsedByChat(t *testing.T) {
	app, identities := newFileCenterTestHandler(t)
	aliceToken := fileCenterToken(t, app, "alice", identities["alice"])
	bobToken := fileCenterToken(t, app, "bob", identities["bob"])
	createTestKnowledgeBase(t, app, "shared-kb", "alice", identities["alice"], "shared")

	body, contentType := makeKnowledgeDocumentUpload(
		t, "shared-kb", "教学实施方案.txt",
		"本学期课程安排包括教学质量检查、课程考核和学生实践。",
	)
	upload := callFileCenter(t, app, aliceToken, http.MethodPost,
		"/knowledge/manage/documents/upload", body, contentType, "", app.UploadLocalKnowledgeDocuments)
	if upload.Code != http.StatusOK {
		t.Fatalf("knowledge upload status=%d body=%s", upload.Code, upload.Body.String())
	}
	items := decodeFileCenterJSON(t, upload)["data"].([]any)
	if len(items) != 1 { t.Fatalf("expected one knowledge document, got %#v", items) }
	item := items[0].(map[string]any)
	fileID := item["file_id"].(string)
	documentID := item["id"].(string)
	if fileID == "" { t.Fatal("knowledge upload did not return its original file ID") }

	search := callFileCenter(t, app, bobToken, http.MethodGet,
		"/files/search?q=教学质量", nil, "", "", app.SearchFiles)
	searchItems := decodeFileCenterJSON(t, search)["data"].(map[string]any)["items"].([]any)
	if search.Code != http.StatusOK || len(searchItems) != 1 {
		t.Fatalf("shared knowledge file should appear in file center: status=%d body=%s", search.Code, search.Body.String())
	}
	searchItem := searchItems[0].(map[string]any)
	if searchItem["knowledge_bases"] != "shared-kb" {
		t.Fatalf("expected knowledge base name on file record, got %#v", searchItem["knowledge_bases"])
	}

	download := callFileCenter(t, app, bobToken, http.MethodGet,
		"/files/"+fileID+"/download", nil, "", fileID, app.DownloadFile)
	if download.Code != http.StatusOK || !strings.Contains(download.Body.String(), "学生实践") {
		t.Fatalf("a user allowed by the shared KB should download the original file: status=%d body=%s", download.Code, download.Body.String())
	}

	bobIdentity := middleware.Identity{Username: "bob", UserID: identities["bob"], Provider: "local"}
	prompt, sources, err := app.knowledgePromptWithSources(context.Background(), bobIdentity, "shared-kb", "课程安排")
	if err != nil || !strings.Contains(prompt, "课程安排") || !strings.Contains(prompt, "教学实施方案.txt") {
		t.Fatalf("chat RAG did not recognize the uploaded knowledge document: prompt=%q err=%v", prompt, err)
	}
	if len(sources) != 1 || sources[0].FileID != fileID || sources[0].DocumentID != documentID ||
		sources[0].BaseName != "shared-kb" || !strings.Contains(sources[0].Snippet, "课程安排") {
		t.Fatalf("chat RAG did not return the original file source: %#v", sources)
	}

	blockedDelete := callFileCenter(t, app, aliceToken, http.MethodPost,
		"/files/"+fileID+"/delete", nil, "", fileID, app.DeleteFile)
	if blockedDelete.Code != http.StatusConflict {
		t.Fatalf("a file still used by a KB must not be deleted: status=%d body=%s", blockedDelete.Code, blockedDelete.Body.String())
	}

	deleteDoc := callFileCenter(t, app, aliceToken, http.MethodPost,
		"/knowledge/manage/documents/delete", strings.NewReader("{\"id\":\""+documentID+"\"}"),
		"application/json", "", app.DeleteLocalKnowledgeDocument)
	if deleteDoc.Code != http.StatusOK {
		t.Fatalf("remove KB document status=%d body=%s", deleteDoc.Code, deleteDoc.Body.String())
	}
	var fileCount int
	if err := app.DB.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM platform_file WHERE id=?", fileID).Scan(&fileCount); err != nil {
		t.Fatal(err)
	}
	if fileCount != 1 {
		t.Fatal("removing a knowledge-base reference must keep the original file in the file center")
	}
}

func TestFileCenterUploadCanBeAttachedToKnowledgeBase(t *testing.T) {
	app, identities := newFileCenterTestHandler(t)
	aliceToken := fileCenterToken(t, app, "alice", identities["alice"])
	createTestKnowledgeBase(t, app, "alice-kb", "alice", identities["alice"], "private")

	body, contentType := makeFileUpload(t, "学生管理制度.txt", "学生管理制度包含考勤、奖惩、请假与学籍管理办法。", "private")
	upload := callFileCenter(t, app, aliceToken, http.MethodPost, "/files/upload", body, contentType, "", app.UploadFiles)
	if upload.Code != http.StatusOK {
		t.Fatalf("file center upload status=%d body=%s", upload.Code, upload.Body.String())
	}
	fileID := decodeFileCenterJSON(t, upload)["data"].(map[string]any)["items"].([]any)[0].(map[string]any)["id"].(string)

	attach := callFileCenter(t, app, aliceToken, http.MethodPost,
		"/knowledge/manage/documents/attach-file",
		strings.NewReader("{\"base_id\":\"alice-kb\",\"file_id\":\""+fileID+"\"}"),
		"application/json", "", app.AttachFileToKnowledgeBase)
	if attach.Code != http.StatusOK {
		t.Fatalf("attach file status=%d body=%s", attach.Code, attach.Body.String())
	}
	aliceIdentity := middleware.Identity{Username: "alice", UserID: identities["alice"], Provider: "local"}
	prompt, err := app.knowledgePrompt(context.Background(), aliceIdentity, "alice-kb", "请假")
	if err != nil || !strings.Contains(prompt, "请假") || !strings.Contains(prompt, "学生管理制度.txt") {
		t.Fatalf("attached file was not indexed for chat: prompt=%q err=%v", prompt, err)
	}
}
