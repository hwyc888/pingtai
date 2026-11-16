package handler

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"strings"
	"testing"

	"pingtai/internal/config"
	"pingtai/internal/repository"

	_ "modernc.org/sqlite"
)

func TestSplitKnowledgeTextCreatesOverlappingChunks(t *testing.T) {
	content := strings.Repeat("知识", 1100)
	chunks := splitKnowledgeText(content)
	if len(chunks) < 3 {
		t.Fatalf("expected at least 3 chunks, got %d", len(chunks))
	}
	first := []rune(chunks[0])
	second := []rune(chunks[1])
	if len(first) < 120 || len(second) < 120 {
		t.Fatal("chunks are unexpectedly short")
	}
	if string(first[len(first)-120:]) != string(second[:120]) {
		t.Fatal("adjacent chunks should preserve the configured overlap")
	}
}

func TestKnowledgeTermsIncludesChineseAndEnglishTerms(t *testing.T) {
	terms := knowledgeTerms("什么是知识库？ OpenAI model")
	for _, term := range []string{"什么", "知识", "知识库", "openai", "model"} {
		if _, ok := terms[term]; !ok {
			t.Fatalf("expected term %q in %#v", term, terms)
		}
	}
}

func TestExtractDOCXTextReadsParagraphs(t *testing.T) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	file, err := writer.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	xml := "<w:document xmlns:w=\"urn:w\"><w:body><w:p><w:r><w:t>第一段</w:t></w:r></w:p><w:p><w:r><w:t>第二段</w:t></w:r></w:p></w:body></w:document>"
	if _, err := file.Write([]byte(xml)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	content, err := extractDOCXText(buffer.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "第一段") || !strings.Contains(content, "第二段") || !strings.Contains(content, "\n") {
		t.Fatalf("unexpected DOCX text: %q", content)
	}
}

func TestBoolFromValue(t *testing.T) {
	if !boolFromValue("true", false) || boolFromValue(float64(0), true) || !boolFromValue(true, false) {
		t.Fatal("boolean values were not parsed correctly")
	}
}

func TestKnowledgeBaseOwnerIsolation(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	_, err = db.Exec(`CREATE TABLE ai_user (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT NOT NULL UNIQUE,
		password TEXT NOT NULL DEFAULT '',
		usertype INTEGER NOT NULL DEFAULT 0,
		status INTEGER NOT NULL DEFAULT 1,
		deltag INTEGER NOT NULL DEFAULT 0,
		openid TEXT DEFAULT NULL,
		bind_time DATETIME DEFAULT NULL,
		created_at DATETIME DEFAULT NULL,
		updated_at DATETIME DEFAULT NULL
	)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE ai_knowledge_base (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL DEFAULT '',
		enabled INTEGER NOT NULL DEFAULT 1,
		created_by TEXT NOT NULL DEFAULT ''
	)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE ai_knowledge_base_policy (
		base_id TEXT PRIMARY KEY,
		owner_username TEXT NOT NULL,
		access_mode TEXT NOT NULL
	)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE ai_knowledge_base_member (
		base_id TEXT NOT NULL,
		username TEXT NOT NULL,
		PRIMARY KEY (base_id, username)
	)`)
	if err != nil {
		t.Fatal(err)
	}
	for _, user := range []struct {
		name string
		role int
	}{
		{name: "alice", role: 0},
		{name: "bob", role: 0},
		{name: "admin", role: 1},
	} {
		if _, err := db.Exec("INSERT INTO ai_user (username,usertype,status,deltag) VALUES (?,?,1,0)", user.name, user.role); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec("INSERT INTO ai_knowledge_base (id,created_by) VALUES ('kb-alice','alice'),('kb-bob','bob')"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO ai_knowledge_base_policy (base_id,owner_username,access_mode) VALUES ('kb-alice','alice','private'),('kb-bob','bob','private')"); err != nil {
		t.Fatal(err)
	}

	h := &Handler{
		Config: &config.Config{},
		DB: db,
		Users: repository.NewUserRepository(db),
	}
	ctx := context.Background()
	cases := []struct {
		username string
		baseID string
		want bool
	}{
		{username: "alice", baseID: "kb-alice", want: true},
		{username: "alice", baseID: "kb-bob", want: false},
		{username: "bob", baseID: "kb-alice", want: false},
		{username: "bob", baseID: "kb-bob", want: true},
		{username: "admin", baseID: "kb-alice", want: true},
		{username: "admin", baseID: "kb-bob", want: true},
		{username: "alice", baseID: "missing", want: false},
	}
	for _, tc := range cases {
		got, err := h.canAccessKnowledgeBase(ctx, tc.username, tc.baseID)
		if err != nil {
			t.Fatalf("%s accessing %s: %v", tc.username, tc.baseID, err)
		}
		if got != tc.want {
			t.Errorf("%s access to %s = %v, want %v", tc.username, tc.baseID, got, tc.want)
		}
	}
	if _, err := h.knowledgePrompt(ctx, "alice", "kb-bob", "private material"); err == nil || err.Error() != "所选知识库不存在或无权访问" {
		t.Fatalf("a user must not retrieve another user's knowledge base, got error: %v", err)
	}
}
