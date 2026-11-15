package handler

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
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
