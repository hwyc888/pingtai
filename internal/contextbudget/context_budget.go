package contextbudget

import (
	"encoding/json"
	"errors"
	"math"
	"unicode"
)

const (
	messageOverheadTokens int64 = 8
	imageReserveTokens    int64 = 4096
	maxDefaultOutput      int64 = 4096
)

var ErrCurrentMessageTooLarge = errors.New("当前问题、附件和系统提示词已超过模型可用上下文预算，请缩短内容或移除附件后重试")

type Result struct {
	Messages             []map[string]any
	ContextWindowTokens  int64
	InputBudgetTokens    int64
	EstimatedInputTokens int64
	ReservedOutputTokens int64
	AllowedOutputTokens  int64
	KeptTurns            int
	DroppedTurns         int
}

// Fit keeps the newest contiguous suffix of complete historical turns.
// A zero context window means the provider did not publish trustworthy metadata;
// in that case the caller's existing turn-count limit remains the only limit.
func Fit(systemPrompt string, history []map[string]any, current any, contextWindow int64, maxOutputTokens int) (Result, error) {
	result := Result{ContextWindowTokens: contextWindow}
	base := make([]map[string]any, 0, 2)
	baseTokens := int64(0)
	if systemPrompt != "" {
		message := map[string]any{"role": "system", "content": systemPrompt}
		base = append(base, message)
		baseTokens += estimateMessage(message)
	}
	currentMessage := map[string]any{"role": "user", "content": current}
	base = append(base, currentMessage)
	baseTokens += estimateMessage(currentMessage)

	if contextWindow <= 0 {
		result.Messages = make([]map[string]any, 0, len(history)+len(base))
		result.Messages = append(result.Messages, base[:len(base)-1]...)
		for _, item := range history {
			result.Messages = append(result.Messages, publicMessage(item))
		}
		result.Messages = append(result.Messages, currentMessage)
		result.EstimatedInputTokens = estimateMessages(result.Messages)
		result.KeptTurns = len(groupTurns(history))
		return result, nil
	}

	outputReserve := defaultOutputReserve(contextWindow)
	if maxOutputTokens > 0 {
		outputReserve = int64(maxOutputTokens)
		if outputReserve > contextWindow/2 {
			outputReserve = contextWindow / 2
		}
	}
	safetyMargin := contextWindow / 20
	if safetyMargin < 64 {
		safetyMargin = 64
	}
	inputBudget := contextWindow - outputReserve - safetyMargin
	result.ReservedOutputTokens = outputReserve
	result.InputBudgetTokens = inputBudget
	if inputBudget <= 0 || baseTokens > inputBudget {
		return result, ErrCurrentMessageTooLarge
	}

	turns := groupTurns(history)
	selected := make([][]map[string]any, 0, len(turns))
	used := baseTokens
	for index := len(turns) - 1; index >= 0; index-- {
		cost := estimateMessages(turns[index])
		if used+cost > inputBudget {
			break
		}
		selected = append(selected, turns[index])
		used += cost
	}
	result.KeptTurns = len(selected)
	result.DroppedTurns = len(turns) - len(selected)
	result.Messages = make([]map[string]any, 0, len(history)+len(base))
	if systemPrompt != "" {
		result.Messages = append(result.Messages, base[0])
	}
	for index := len(selected) - 1; index >= 0; index-- {
		for _, item := range selected[index] {
			result.Messages = append(result.Messages, publicMessage(item))
		}
	}
	result.Messages = append(result.Messages, currentMessage)
	result.EstimatedInputTokens = used
	if maxOutputTokens > 0 {
		availableOutput := contextWindow - used - safetyMargin
		result.AllowedOutputTokens = int64(maxOutputTokens)
		if result.AllowedOutputTokens > availableOutput {
			result.AllowedOutputTokens = availableOutput
		}
	}
	return result, nil
}

func defaultOutputReserve(contextWindow int64) int64 {
	reserve := contextWindow / 8
	if reserve < 256 {
		reserve = 256
	}
	if reserve > maxDefaultOutput {
		reserve = maxDefaultOutput
	}
	if reserve > contextWindow/2 {
		reserve = contextWindow / 2
	}
	return reserve
}

func groupTurns(history []map[string]any) [][]map[string]any {
	turns := make([][]map[string]any, 0)
	turnIndexes := make(map[string]int)
	for _, item := range history {
		turnID, _ := item["turn_id"].(string)
		if turnID != "" {
			index, exists := turnIndexes[turnID]
			if !exists {
				index = len(turns)
				turnIndexes[turnID] = index
				turns = append(turns, make([]map[string]any, 0, 2))
			}
			turns[index] = append(turns[index], item)
			continue
		}
		role, _ := item["role"].(string)
		if role == "user" && len(turns) > 0 && len(turns[len(turns)-1]) > 0 {
			turns = append(turns, make([]map[string]any, 0, 2))
		}
		if len(turns) == 0 {
			turns = append(turns, make([]map[string]any, 0, 2))
		}
		turns[len(turns)-1] = append(turns[len(turns)-1], item)
	}
	return turns
}

func publicMessage(item map[string]any) map[string]any {
	return map[string]any{
		"role": item["role"],
		"content": item["content"],
	}
}

func estimateMessages(messages []map[string]any) int64 {
	total := int64(0)
	for _, message := range messages {
		total += estimateMessage(message)
	}
	return total
}

func estimateMessage(message map[string]any) int64 {
	return messageOverheadTokens + estimateValue(message["content"])
}

func estimateValue(value any) int64 {
	switch item := value.(type) {
	case nil:
		return 0
	case string:
		return EstimateTextTokens(item)
	case json.RawMessage:
		var decoded any
		if json.Unmarshal(item, &decoded) == nil {
			return estimateValue(decoded)
		}
		return EstimateTextTokens(string(item))
	case []byte:
		return EstimateTextTokens(string(item))
	case []any:
		total := int64(0)
		for _, child := range item {
			total += estimateValue(child)
		}
		return total
	case []map[string]any:
		total := int64(0)
		for _, child := range item {
			total += estimateValue(child)
		}
		return total
	case map[string]any:
		kind, _ := item["type"].(string)
		if kind == "image" || kind == "image_url" || item["image_url"] != nil {
			return imageReserveTokens + estimateValue(item["text"])
		}
		if text, exists := item["text"]; exists {
			return estimateValue(text)
		}
		if content, exists := item["content"]; exists {
			return estimateValue(content)
		}
		total := int64(0)
		for key, child := range item {
			if key == "url" || key == "file_id" || key == "id" {
				continue
			}
			total += estimateValue(child)
		}
		return total
	case json.Number:
		return EstimateTextTokens(item.String())
	case float64, int, int64, uint, uint64, bool:
		return 1
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return 0
		}
		return EstimateTextTokens(string(encoded))
	}
}

// EstimateTextTokens is a deliberately conservative, model-agnostic estimate.
// It overestimates ordinary ASCII text and CJK text rather than pretending to be
// an exact tokenizer for every OpenAI-compatible provider.
func EstimateTextTokens(value string) int64 {
	var asciiRun int64
	var cjkRunes int64
	var otherRunes int64
	flushASCII := func() int64 {
		if asciiRun == 0 {
			return 0
		}
		count := int64(math.Ceil(float64(asciiRun) / 3.0))
		asciiRun = 0
		return count
	}
	total := int64(0)
	for _, character := range value {
		if character < 128 {
			asciiRun++
			continue
		}
		total += flushASCII()
		switch {
		case unicode.Is(unicode.Han, character),
			unicode.Is(unicode.Hiragana, character),
			unicode.Is(unicode.Katakana, character),
			unicode.Is(unicode.Hangul, character):
			cjkRunes++
		default:
			otherRunes++
		}
	}
	total += flushASCII()
	total += int64(math.Ceil(float64(cjkRunes) * 1.5))
	total += otherRunes * 2
	return total
}
