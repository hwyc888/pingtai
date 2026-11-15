package handler

import (
	"bufio"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"pingtai/internal/contextbudget"
	"pingtai/internal/model"
	"pingtai/internal/service/openai"
	"pingtai/internal/util"
)

func (h *Handler) Chat(w http.ResponseWriter, r *http.Request) {
	if !h.parseForm(w, r) {
		return
	}
	if h.Config.UsesLocalConversations() {
		h.chatModel(w, r)
		return
	}
	h.chatCoze(w, r)
}

func (h *Handler) chatCoze(w http.ResponseWriter, r *http.Request) {
	conversationID := strings.TrimSpace(r.FormValue("conversation_id"))
	botID := strings.TrimSpace(r.FormValue("bot_id"))
	content := r.FormValue("content")
	if conversationID == "" || botID == "" || content == "" {
		writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "缺少会话ID、智能体ID或消息内容"})
		return
	}
	identity, _ := currentIdentity(r)
	stream := r.FormValue("stream") == "1"
	payload := map[string]any{
		"bot_id": botID, "user_id": identity.Username,
		"stream": stream, "auto_save_history": true,
	}
	file, header, fileErr := r.FormFile("file")
	if fileErr == nil {
		defer file.Close()
	}
	imageURL := strings.TrimSpace(r.FormValue("url"))
	if imageURL == "" && fileErr != nil {
		payload["additional_messages"] = []map[string]any{{
			"role": "user", "content_type": "text", "content": content,
		}}
	} else {
		upType := r.FormValue("up_type")
		if upType != "image" && upType != "file" {
			writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "缺少或不支持上传类型参数"})
			return
		}
		attachment := map[string]any{"type": upType}
		if imageURL != "" {
			attachment["file_url"] = imageURL
		} else {
			fileID, err := h.uploadCozeFile(r.Context(), file, header)
			if err != nil {
				writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "上传文件失败: " + err.Error()})
				return
			}
			attachment["file_id"] = fileID
		}
		objectContent, _ := json.Marshal([]any{
			attachment,
			map[string]any{"type": "text", "text": content},
		})
		payload["additional_messages"] = []map[string]any{{
			"role": "user", "content_type": "object_string", "content": string(objectContent),
		}}
	}
	query := url.Values{"conversation_id": []string{conversationID}}
	if !stream {
		h.callCozeJSON(w, r, http.MethodPost, "/v3/chat", query, payload, nil)
		return
	}

	body, err := json.Marshal(payload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "序列化请求失败")
		return
	}
	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	resp, err := h.Coze.Do(r.Context(), http.MethodPost, "/v3/chat", query, body, headers)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"code": 502, "msg": err.Error(), "error": true})
		return
	}
	if resp.StatusCode >= 400 {
		h.writeCozeResponse(w, resp)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	reader := bufio.NewReader(resp.Body)
	for {
		line, readErr := reader.ReadString('\n')
		if strings.TrimSpace(line) != "" {
			encoded, _ := json.Marshal(map[string]any{"code": 200, "msg": "success", "data": strings.TrimRight(line, "\r\n")})
			_, _ = w.Write(append(encoded, '\n'))
			if flusher != nil {
				flusher.Flush()
			}
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				return
			}
			break
		}
	}
}

func (h *Handler) chatModel(w http.ResponseWriter, r *http.Request) {
	conversationID := strings.TrimSpace(r.FormValue("conversation_id"))
	botID := strings.TrimSpace(r.FormValue("bot_id"))
	content := r.FormValue("content")
	if conversationID == "" || botID == "" || content == "" {
		writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "缺少会话ID、智能体ID或消息内容"})
		return
	}
	activeConfig, _, providerErr := h.activeChatProvider(r.Context())
	if errors.Is(providerErr, sql.ErrNoRows) || errors.Is(providerErr, errNoActiveChatProvider) {
		writeJSON(w, http.StatusOK, map[string]any{"code": 503, "msg": "当前没有配置模型渠道，请联系管理员配置"})
		return
	}
	if providerErr != nil {
		writeError(w, http.StatusInternalServerError, "读取当前模型供应商失败")
		return
	}
	selectedModel, allowed := selectChatModel(&activeConfig, botID)
	if !allowed {
		writeJSON(w, http.StatusOK, map[string]any{"code": 400, "msg": "所选模型已停用或不存在，请刷新模型列表后重试"})
		return
	}
	identity, _ := currentIdentity(r)
	_, err := h.Chats.FindConversation(r.Context(), conversationID, identity.Username)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"code": 404, "msg": "会话不存在"})
		return
	}
	_ = h.Chats.UpdateConversationBot(r.Context(), conversationID, identity.Username, botID)

	userContent, storedContent, err := h.buildModelUserContent(r, content)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"code": 400, "msg": err.Error()})
		return
	}
	history, err := h.Chats.ContextMessages(r.Context(), conversationID, activeConfig.ContextTurns)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取会话历史失败")
		return
	}

	systemPrompt := activeConfig.SystemPrompt()
	if knowledgeBaseID := strings.TrimSpace(r.FormValue("knowledge_base_id")); knowledgeBaseID != "" {
		knowledgeContext, knowledgeErr := h.knowledgePrompt(r.Context(), knowledgeBaseID, content)
		if knowledgeErr != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"code": http.StatusBadRequest, "msg": knowledgeErr.Error()})
			return
		}
		systemPrompt = strings.TrimSpace(systemPrompt + "\n\n" + knowledgeContext)
	}
	budget, err := contextbudget.Fit(
		systemPrompt, history, userContent,
		activeConfig.ModelContextTokens, activeConfig.OpenAIMaxTokens,
	)
	if err != nil {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{
			"code": http.StatusRequestEntityTooLarge,
			"msg": "当前问题、附件和系统提示词已超过模型可用上下文预算，请缩短内容或移除附件后重试",
		})
		return
	}
	if budget.DroppedTurns > 0 {
		log.Printf("context budget: model=%s window=%d input=%d kept_turns=%d dropped_turns=%d",
			selectedModel, budget.ContextWindowTokens, budget.EstimatedInputTokens,
			budget.KeptTurns, budget.DroppedTurns)
	}
	chatID, err := util.NewID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "生成聊天ID失败")
		return
	}
	if err := h.Chats.CreateChat(r.Context(), chatID, conversationID, "in_progress"); err != nil {
		writeError(w, http.StatusInternalServerError, "创建聊天记录失败")
		return
	}
	userMessage, err := h.Chats.InsertMessage(r.Context(), conversationID, chatID, "user", storedContent, "")
	if err != nil {
		_ = h.Chats.UpdateChat(r.Context(), chatID, "failed", err.Error())
		writeError(w, http.StatusInternalServerError, "保存用户消息失败")
		return
	}
	if _, plainText := userContent.(string); !plainText {
		if err := h.Chats.SaveMessageContent(r.Context(), userMessage.ID, userContent); err != nil {
			_ = h.Chats.UpdateChat(r.Context(), chatID, "failed", "cannot save multimodal context")
			writeError(w, http.StatusInternalServerError, "保存附件上下文失败")
			return
		}
	}

	messages := budget.Messages

	payload := map[string]any{
		"model":    selectedModel,
		"messages": messages,
		"stream":   false,
	}
	if activeConfig.ChatProvider == "openai" && activeConfig.OpenAIMaxTokens > 0 {
		outputLimit := int64(activeConfig.OpenAIMaxTokens)
		if budget.AllowedOutputTokens > 0 && budget.AllowedOutputTokens < outputLimit {
			outputLimit = budget.AllowedOutputTokens
		}
		if outputLimit < 1 {
			_ = h.Chats.UpdateChat(r.Context(), chatID, "failed", "model context has no output token budget")
			writeError(w, http.StatusRequestEntityTooLarge, "当前上下文已占满模型窗口，请缩短历史内容后重试")
			return
		}
		payload["max_tokens"] = int(outputLimit)
	}
	body, status, callErr := openai.NewClient(activeConfig).Complete(r.Context(), payload)
	if callErr != nil || status < 200 || status >= 300 {
		message := "模型调用失败"
		if callErr != nil {
			message = callErr.Error()
		} else if len(body) > 0 {
			message = string(body)
		}
		_ = h.Chats.UpdateChat(r.Context(), chatID, "failed", message)
		_, _ = h.Chats.InsertMessage(r.Context(), conversationID, chatID, "assistant", "模型调用失败，请检查模型接口配置。", "")
		writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "success", "data": map[string]any{"id": chatID}})
		return
	}

	answer, reasoning, err := parseModelAnswer(body)
	if err != nil {
		_ = h.Chats.UpdateChat(r.Context(), chatID, "failed", err.Error())
		_, _ = h.Chats.InsertMessage(r.Context(), conversationID, chatID, "assistant", "模型响应解析失败。", "")
		writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "success", "data": map[string]any{"id": chatID}})
		return
	}
	if _, err := h.Chats.InsertMessage(r.Context(), conversationID, chatID, "assistant", answer, reasoning); err != nil {
		_ = h.Chats.UpdateChat(r.Context(), chatID, "failed", err.Error())
		writeError(w, http.StatusInternalServerError, "保存模型回复失败")
		return
	}
	if err := h.Chats.UpdateChat(r.Context(), chatID, "completed", ""); err != nil {
		writeError(w, http.StatusInternalServerError, "保存会话完成状态失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"code": 0, "msg": "success", "data": map[string]any{"id": chatID},
	})
}

func (h *Handler) buildModelUserContent(r *http.Request, content string) (any, string, error) {
	imageURL := strings.TrimSpace(r.FormValue("url"))
	file, header, fileErr := r.FormFile("file")
	if fileErr == nil {
		defer file.Close()
	}
	upType := r.FormValue("up_type")
	if imageURL == "" && fileErr != nil {
		return content, content, nil
	}
	if upType == "image" {
		imageSource := imageURL
		if imageSource == "" {
			data, err := io.ReadAll(io.LimitReader(file, h.Config.MaxUploadBytes+1))
			if err != nil {
				return nil, "", err
			}
			if int64(len(data)) > h.Config.MaxUploadBytes {
				return nil, "", errors.New("图片超过大小限制")
			}
			extension := strings.ToLower(filepath.Ext(header.Filename))
			contentType := mime.TypeByExtension(extension)
			if !strings.HasPrefix(contentType, "image/") {
				return nil, "", errors.New("图片格式不受支持")
			}
			imageSource = "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(data)
		}
		parts := []map[string]any{
			{"type": "text", "text": content},
			{"type": "image_url", "image_url": map[string]any{"url": imageSource}},
		}
		return parts, content + " [图片附件]", nil
	}
	if upType == "file" {
		if fileErr != nil {
			return nil, "", errors.New("模型直连模式下文件链接暂不支持，请上传 TXT 文件或切换 CHAT_PROVIDER=coze")
		}
		if strings.ToLower(filepath.Ext(header.Filename)) != ".txt" {
			return nil, "", errors.New("模型直连模式目前支持 TXT 文件；PDF/DOC 文件请使用 Coze 模式")
		}
		data, err := io.ReadAll(io.LimitReader(file, h.Config.MaxUploadBytes+1))
		if err != nil {
			return nil, "", err
		}
		if int64(len(data)) > h.Config.MaxUploadBytes {
			return nil, "", errors.New("文件超过大小限制")
		}
		text := content + "\n\n附件 " + header.Filename + "：\n" + string(data)
		return text, text, nil
	}
	return nil, "", errors.New("缺少或不支持上传类型参数")
}

func parseModelAnswer(body []byte) (string, string, error) {
	var response struct {
		Choices []struct {
			Message struct {
				Content          json.RawMessage `json:"content"`
				ReasoningContent string          `json:"reasoning_content"`
				Refusal          string          `json:"refusal"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", "", err
	}
	if len(response.Choices) == 0 {
		return "", "", errors.New("模型响应中没有 choices")
	}
	message := response.Choices[0].Message
	var answer string
	if err := json.Unmarshal(message.Content, &answer); err != nil {
		var parts []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(message.Content, &parts); err != nil {
			return "", "", errors.New("模型响应 content 格式不支持")
		}
		for _, part := range parts {
			if part.Type == "text" || part.Type == "output_text" {
				answer += part.Text
			}
		}
	}
	if answer == "" && message.Refusal != "" {
		answer = message.Refusal
	}
	if strings.TrimSpace(answer) == "" {
		return "", "", errors.New("模型没有返回文本；网页聊天不执行工具调用，请使用标准 API 处理工具调用")
	}
	return answer, message.ReasoningContent, nil
}

func formatMessage(item model.Message) map[string]any {
	return map[string]any{
		"id": item.ID, "role": item.Role, "content": item.Content,
		"reasoning_content": item.ReasoningContent, "created_at": item.CreatedAt,
	}
}
