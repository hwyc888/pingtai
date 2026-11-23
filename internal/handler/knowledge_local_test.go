package handler

import (
	"archive/zip"
	"bytes"
	"context"
	"strings"
	"testing"

	"pingtai/internal/config"
	"pingtai/internal/database"
	"pingtai/internal/middleware"
	"pingtai/internal/repository"

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

func TestExtractOfficeOpenXMLTextSupportsSpreadsheetAndSlides(t *testing.T) {
	makeArchive := func(path, xmlContent string) []byte {
		var buffer bytes.Buffer
		writer := zip.NewWriter(&buffer)
		file, err := writer.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(xmlContent)); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		return buffer.Bytes()
	}

	spreadsheet := makeArchive("xl/sharedStrings.xml", "<sst><si><t>教学计划与课程安排</t></si></sst>")
	spreadsheetText, err := extractKnowledgeText(context.Background(), "课程表.xlsx", spreadsheet)
	if err != nil || !strings.Contains(spreadsheetText, "课程安排") {
		t.Fatalf("xlsx text extraction failed: text=%q err=%v", spreadsheetText, err)
	}

	presentation := makeArchive("ppt/slides/slide1.xml", "<p:sld xmlns:a='urn:a'><a:p><a:r><a:t>年度工作汇报</a:t></a:r></a:p></p:sld>")
	presentationText, err := extractKnowledgeText(context.Background(), "工作汇报.pptx", presentation)
	if err != nil || !strings.Contains(presentationText, "年度工作汇报") {
		t.Fatalf("pptx text extraction failed: text=%q err=%v", presentationText, err)
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
	ctx := context.Background()
	db, err := database.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}

	users := repository.NewUserRepository(db)
	identities := make(map[string]string)
	for _, item := range []struct {
		name string
		role int
	}{
		{name: "alice", role: 0},
		{name: "bob", role: 0},
		{name: "admin", role: 1},
	} {
		id, err := users.Create(ctx, item.name, "test-hash", item.role)
		if err != nil {
			t.Fatal(err)
		}
		user, err := users.FindByID(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		identity, err := users.EnsureLocalIdentity(ctx, user)
		if err != nil {
			t.Fatal(err)
		}
		identities[item.name] = identity.ID
	}
	for _, item := range []struct {
		id, owner string
	}{
		{"kb-alice", "alice"}, {"kb-bob", "bob"},
	} {
		if _, err := db.ExecContext(ctx,
			"INSERT INTO ai_knowledge_base (id,name,description,enabled,created_by,created_at,updated_at) VALUES (?,?, '',1,?,'now','now')",
			item.id, item.id, item.owner,
		); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx,
			"INSERT INTO ai_knowledge_base_policy (base_id,owner_username,access_mode) VALUES (?,?,'private')",
			item.id, item.owner,
		); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx,
			"INSERT INTO ai_knowledge_base_identity (base_id,owner_user_id,access_mode) VALUES (?,?,'private')",
			item.id, identities[item.owner],
		); err != nil {
			t.Fatal(err)
		}
	}

	cfg := &config.Config{}
	h := &Handler{Config: cfg, DB: db, Users: users}
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
		identity := middleware.Identity{Username: tc.username, UserID: identities[tc.username], Provider: "local"}
		got, err := h.canAccessKnowledgeBase(ctx, identity, tc.baseID)
		if err != nil {
			t.Fatalf("%s accessing %s: %v", tc.username, tc.baseID, err)
		}
		if got != tc.want {
			t.Errorf("%s access to %s = %v, want %v", tc.username, tc.baseID, got, tc.want)
		}
	}
	aliceIdentity := middleware.Identity{Username:"alice",UserID:identities["alice"],Provider:"local"}
	if _, err := h.knowledgePrompt(ctx, aliceIdentity, "kb-bob", "private material"); err == nil || err.Error() != "所选知识库不存在或无权访问" {
		t.Fatalf("a user must not retrieve another user's knowledge base, got error: %v", err)
	}
}
