package contextbudget

import (
	"errors"
	"strings"
	"testing"
)

func turn(id string, userText, assistantText string) []map[string]any {
	return []map[string]any{
		{"turn_id": id, "role": "user", "content": userText},
		{"turn_id": id, "role": "assistant", "content": assistantText},
	}
}

func TestFitKeepsNewestCompleteTurnsWithinBudget(t *testing.T) {
	history := make([]map[string]any, 0, 6)
	for _, id := range []string{"old", "middle", "newest"} {
		history = append(history, turn(id, strings.Repeat("u", 1000), strings.Repeat("a", 1000))...)
	}
	result, err := Fit("system", history, "current question", 4096, 3072)
	if err != nil {
		t.Fatal(err)
	}
	if result.KeptTurns != 2 || result.DroppedTurns != 1 {
		t.Fatalf("kept=%d dropped=%d", result.KeptTurns, result.DroppedTurns)
	}
	if len(result.Messages) != 6 {
		t.Fatalf("expected system + two complete turns + current question, got %d messages", len(result.Messages))
	}
	if result.Messages[1]["content"] != strings.Repeat("u", 1000) || result.Messages[3]["content"] != strings.Repeat("u", 1000) {
		t.Fatal("the two newest turns were not preserved in chronological order")
	}
	for _, message := range result.Messages {
		if _, exists := message["turn_id"]; exists {
			t.Fatal("internal turn IDs must not be sent to the upstream model")
		}
	}
	if result.EstimatedInputTokens > result.InputBudgetTokens {
		t.Fatalf("estimated input %d exceeded budget %d", result.EstimatedInputTokens, result.InputBudgetTokens)
	}
	if result.AllowedOutputTokens <= 0 || result.AllowedOutputTokens >= 3072 {
		t.Fatalf("output limit should be reduced to fit the remaining context window, got %d", result.AllowedOutputTokens)
	}
}

func TestFitNeverSplitsAnOversizedTurn(t *testing.T) {
	history := turn("large", strings.Repeat("u", 400), strings.Repeat("a", 4000))
	result, err := Fit("", history, "new question", 2048, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if result.KeptTurns != 0 || result.DroppedTurns != 1 {
		t.Fatalf("oversized turn should be dropped as a whole: %#v", result)
	}
	if len(result.Messages) != 1 || result.Messages[0]["content"] != "new question" {
		t.Fatalf("current question was not preserved: %#v", result.Messages)
	}
}

func TestFitRejectsCurrentMessageThatCannotFit(t *testing.T) {
	_, err := Fit("system", nil, strings.Repeat("x", 3000), 1024, 512)
	if !errors.Is(err, ErrCurrentMessageTooLarge) {
		t.Fatalf("expected oversized current message error, got %v", err)
	}
}

func TestFitWithoutContextMetadataDoesNotInventALimit(t *testing.T) {
	history := turn("one", "older question", "older answer")
	result, err := Fit("", history, "current question", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if result.KeptTurns != 1 || result.DroppedTurns != 0 || len(result.Messages) != 3 {
		t.Fatalf("history should remain intact when context metadata is unknown: %#v", result)
	}
}

func TestImageContentUsesFixedReserveInsteadOfCountingBase64(t *testing.T) {
	content := []map[string]any{
		{"type": "text", "text": "describe this"},
		{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + strings.Repeat("A", 500000)}},
	}
	tokens := estimateValue(content)
	if tokens < imageReserveTokens || tokens > imageReserveTokens+20 {
		t.Fatalf("image content should use a bounded image reserve, got %d tokens", tokens)
	}
}

func TestEstimateTextTokensCountsCJKAndASCII(t *testing.T) {
	if EstimateTextTokens("hello world") == 0 {
		t.Fatal("ASCII text should have a positive token estimate")
	}
	if EstimateTextTokens("你好，世界") < 7 {
		t.Fatal("CJK text should be estimated conservatively")
	}
}
