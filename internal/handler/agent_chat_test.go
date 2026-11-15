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
