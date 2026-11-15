package handler

import "net/http"

func (h *Handler) KnowledgeStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"code": 0,
		"data": map[string]any{
			"available": h.Coze != nil && h.Coze.Configured(),
			"provider": "coze",
			"workspace_id": h.Config.CozeWorkspaceID,
			"message": "知识库文档管理当前使用 Coze 知识库 API；OpenAI 兼容聊天供应商本身不提供知识库存储。",
		},
	})
}
