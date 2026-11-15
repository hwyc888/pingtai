package handler

import "testing"

func TestParseProviderModelsReadsOptionalContextMetadata(t *testing.T) {
	body := []byte("{\"data\":[{\"id\":\"model-a\",\"context_length\":131072,\"max_output_tokens\":8192},{\"id\":\"model-b\"},{\"id\":\"model-a\"}]}")
	models, err := parseProviderModels(body, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 {
		t.Fatalf("expected duplicate model IDs to be removed, got %d", len(models))
	}
	if models[0].ModelID != "model-a" || models[0].ContextLength == nil || *models[0].ContextLength != 131072 {
		t.Fatalf("context metadata was not parsed: %#v", models[0])
	}
	if models[1].ContextLength != nil {
		t.Fatalf("missing upstream context metadata should remain unknown: %#v", models[1])
	}
}
