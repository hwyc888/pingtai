package handler

import "testing"

func TestParseModelAnswer(t *testing.T) {
	body := []byte(`{"choices":[{"message":{"content":"hello","reasoning_content":"thinking"}}]}`)
	answer, reasoning, err := parseModelAnswer(body)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "hello" || reasoning != "thinking" {
		t.Fatalf("unexpected answer=%q reasoning=%q", answer, reasoning)
	}
}

func TestParseModelAnswerVariants(t *testing.T) {
	for _, test := range []struct {
		name   string
		body   string
		answer string
		fails  bool
	}{
		{name: "content parts", body: `{"choices":[{"message":{"content":[{"type":"text","text":"first"},{"type":"text","text":"second"}]}}]}`, answer: "firstsecond"},
		{name: "refusal", body: `{"choices":[{"message":{"content":null,"refusal":"Cannot answer"}}]}`, answer: "Cannot answer"},
		{name: "tool-only response", body: `{"choices":[{"message":{"content":null,"tool_calls":[{"id":"call_1"}]}}]}`, fails: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			answer, _, err := parseModelAnswer([]byte(test.body))
			if (err != nil) != test.fails || answer != test.answer {
				t.Fatalf("answer=%q err=%v", answer, err)
			}
		})
	}
}

func TestParseOpenAIStreamChunk(t *testing.T) {
	for _, test := range []struct {
		name      string
		payload   string
		content   string
		reasoning string
		wantError bool
	}{
		{
			name: "text delta",
			payload: `{"choices":[{"delta":{"content":"hello "}}]}`,
			content: "hello ",
		},
		{
			name: "reasoning delta",
			payload: `{"choices":[{"delta":{"reasoning_content":"thinking"}}]}`,
			reasoning: "thinking",
		},
		{
			name: "multimodal text parts",
			payload: `{"choices":[{"delta":{"content":[{"type":"text","text":"one"},{"type":"output_text","text":"two"}]}}]}`,
			content: "onetwo",
		},
		{
			name: "refusal delta",
			payload: `{"choices":[{"delta":{"refusal":"cannot help"}}]}`,
			content: "cannot help",
		},
		{
			name: "provider error",
			payload: `{"error":{"message":"rate limited"}}`,
			wantError: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			content, reasoning, err := parseOpenAIStreamChunk(test.payload)
			if (err != nil) != test.wantError {
				t.Fatalf("unexpected error: %v", err)
			}
			if content != test.content || reasoning != test.reasoning {
				t.Fatalf("content=%q reasoning=%q", content, reasoning)
			}
		})
	}
}
