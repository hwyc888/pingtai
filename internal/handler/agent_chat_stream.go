package handler

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"pingtai/internal/model"
)

// streamModelChat proxies OpenAI-compatible SSE deltas to the browser while
// retaining the accumulated answer so an interrupted generation is not lost.
func (h *Handler) streamModelChat(w http.ResponseWriter, r *http.Request, client interface {
	Do(context.Context, []byte) (*http.Response, error)
}, chatID, conversationID string, userMessageID int64, body []byte) {
	h.streamModelChatWithSources(w, r, client, chatID, conversationID, userMessageID, body, nil)
}

func (h *Handler) streamModelChatWithSources(w http.ResponseWriter, r *http.Request, client interface {
	Do(context.Context, []byte) (*http.Response, error)
}, chatID, conversationID string, userMessageID int64, body []byte, knowledgeSources []model.MessageKnowledgeSource) {
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)

	answer := strings.Builder{}
	reasoning := strings.Builder{}
	status := "completed"
	lastError := ""
	clientGone := false

	persist := func(finalStatus, message string) string {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 10*time.Second)
		defer cancel()
		if answer.Len() > 0 || reasoning.Len() > 0 {
			assistantMessage, err := h.Chats.InsertMessage(ctx, conversationID, chatID, "assistant", answer.String(), reasoning.String())
			if err != nil {
				log.Printf("save streamed assistant message %s: %v", chatID, err)
				finalStatus = "failed"
				message = "保存模型回复失败"
			} else if err := h.Chats.SaveMessageKnowledgeSources(ctx, assistantMessage.ID, knowledgeSources); err != nil {
				log.Printf("save streamed knowledge sources %s: %v", chatID, err)
				finalStatus = "failed"
				message = "保存知识库来源失败"
			}
		}
		if err := h.Chats.UpdateChat(ctx, chatID, finalStatus, message); err != nil {
			log.Printf("update streamed chat %s: %v", chatID, err)
		}
		return finalStatus
	}

	emit := func(event string, value any) bool {
		if clientGone {
			return false
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return false
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, encoded); err != nil {
			clientGone = true
			return false
		}
		if flusher != nil {
			flusher.Flush()
		}
		return true
	}

	response, err := client.Do(r.Context(), body)
	if err != nil {
		if r.Context().Err() != nil {
			status = "stopped"
			status = persist(status, "")
			return
		}
		status = "failed"
		lastError = err.Error()
		status = persist(status, lastError)
		emit("error", map[string]any{"message": "模型连接失败：" + err.Error()})
		emit("done", map[string]any{"id": chatID, "status": status})
		return
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		bodyBytes, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		lastError = strings.TrimSpace(string(bodyBytes))
		if lastError == "" {
			lastError = fmt.Sprintf("upstream model returned HTTP %d", response.StatusCode)
		}
		status = "failed"
		status = persist(status, lastError)
		emit("error", map[string]any{"message": "模型调用失败，请检查模型渠道配置。"})
		emit("done", map[string]any{"id": chatID, "status": status})
		return
	}

	if !emit("start", map[string]any{"id": chatID, "user_message_id": userMessageID}) {
		status = "stopped"
		status = persist(status, "")
		return
	}

	if !strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
		bodyBytes, readErr := io.ReadAll(io.LimitReader(response.Body, 32<<20))
		if readErr != nil {
			status = "failed"
			status = persist(status, readErr.Error())
			emit("error", map[string]any{"message": "读取模型响应失败"})
			emit("done", map[string]any{"id": chatID, "status": status})
			return
		}
		answerText, reasoningText, parseErr := parseModelAnswer(bodyBytes)
		if parseErr != nil {
			status = "failed"
			status = persist(status, parseErr.Error())
			emit("error", map[string]any{"message": "模型响应解析失败"})
			emit("done", map[string]any{"id": chatID, "status": status})
			return
		}
		answer.WriteString(answerText)
		reasoning.WriteString(reasoningText)
		emit("delta", map[string]any{"content": answerText})
		if reasoningText != "" {
			emit("reasoning", map[string]any{"content": reasoningText})
		}
		status = persist("completed", "")
		emit("done", map[string]any{"id": chatID, "status": status, "knowledge_sources": knowledgeSources})
		return
	}

	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 64*1024), 32<<20)
	dataLines := make([]string, 0, 2)
	finished := false
	var streamErr error

	consumeEvent := func() {
		if len(dataLines) == 0 || finished || streamErr != nil || clientGone {
			dataLines = dataLines[:0]
			return
		}
		payload := strings.TrimSpace(strings.Join(dataLines, "\n"))
		dataLines = dataLines[:0]
		if payload == "" {
			return
		}
		if payload == "[DONE]" {
			finished = true
			return
		}
		content, thinking, parseErr := parseOpenAIStreamChunk(payload)
		if parseErr != nil {
			streamErr = parseErr
			return
		}
		if content != "" {
			answer.WriteString(content)
			if !emit("delta", map[string]any{"content": content}) {
				clientGone = true
				return
			}
		}
		if thinking != "" {
			reasoning.WriteString(thinking)
			if !emit("reasoning", map[string]any{"content": thinking}) {
				clientGone = true
				return
			}
		}
	}

	for scanner.Scan() {
		line := strings.TrimSpace(strings.TrimSuffix(scanner.Text(), "\r"))
		if line == "" {
			consumeEvent()
			if finished || streamErr != nil || clientGone {
				break
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if len(dataLines) > 0 && !finished && streamErr == nil && !clientGone {
		consumeEvent()
	}
	if scanErr := scanner.Err(); scanErr != nil && r.Context().Err() == nil && !errors.Is(scanErr, context.Canceled) {
		streamErr = scanErr
	}

	switch {
	case r.Context().Err() != nil || clientGone:
		status = "stopped"
		status = persist(status, "")
	case streamErr != nil:
		status = "failed"
		lastError = streamErr.Error()
		status = persist(status, lastError)
		emit("error", map[string]any{"message": "模型流式输出中断：" + lastError})
		emit("done", map[string]any{"id": chatID, "status": status})
	case answer.Len() == 0:
		status = "failed"
		lastError = "模型没有返回文本内容"
		status = persist(status, lastError)
		emit("error", map[string]any{"message": lastError})
		emit("done", map[string]any{"id": chatID, "status": status})
	default:
		status = persist(status, "")
		emit("done", map[string]any{"id": chatID, "status": status, "knowledge_sources": knowledgeSources})
	}
}

func parseOpenAIStreamChunk(payload string) (string, string, error) {
	var chunk struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Choices []struct {
			Delta struct {
				Content          json.RawMessage `json:"content"`
				ReasoningContent string          `json:"reasoning_content"`
				Reasoning        string          `json:"reasoning"`
				Refusal          string          `json:"refusal"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		return "", "", err
	}
	if chunk.Error != nil {
		return "", "", errors.New(chunk.Error.Message)
	}
	if len(chunk.Choices) == 0 {
		return "", "", nil
	}
	delta := chunk.Choices[0].Delta
	content, err := parseStreamContent(delta.Content)
	if err != nil {
		return "", "", err
	}
	if content == "" {
		content = delta.Refusal
	}
	thinking := delta.ReasoningContent
	if thinking == "" {
		thinking = delta.Reasoning
	}
	return content, thinking, nil
}

func parseStreamContent(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text, nil
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return "", err
	}
	var content strings.Builder
	for _, part := range parts {
		if part.Type == "text" || part.Type == "output_text" {
			content.WriteString(part.Text)
		}
	}
	return content.String(), nil
}

func proxyChatEventStream(w http.ResponseWriter, response *http.Response) {
	defer response.Body.Close()
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	reader := bufio.NewReader(response.Body)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			if _, writeErr := w.Write(line); writeErr != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			return
		}
	}
}
