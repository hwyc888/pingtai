package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

const maxKnowledgeChunkRunes = 12000

func validateKnowledgeChunkContent(value string) (string, string) {
	content := strings.TrimSpace(value)
	if content == "" {
		return "", "分块内容不能为空"
	}
	if utf8.RuneCountInString(content) > maxKnowledgeChunkRunes {
		return "", "单个分块最多支持 12000 个字符"
	}
	return content, ""
}

func (h *Handler) ListLocalKnowledgeChunks(w http.ResponseWriter, r *http.Request) {
	documentID := strings.TrimSpace(r.URL.Query().Get("document_id"))
	if documentID == "" {
		writeError(w, http.StatusBadRequest, "缺少文档 ID")
		return
	}
	var baseID string
	if err := h.DB.QueryRowContext(r.Context(),
		"SELECT base_id FROM ai_knowledge_document WHERE id=?", documentID,
	).Scan(&baseID); errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "文档不存在")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "读取文档归属失败")
		return
	}
	if !h.requireKnowledgeBaseAccess(w, r, baseID) {
		return
	}

	rows, err := h.DB.QueryContext(r.Context(),
		"SELECT id,chunk_index,content,created_at FROM ai_knowledge_chunk "+
			"WHERE document_id=? ORDER BY chunk_index ASC,id ASC",
		documentID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取文档分块失败")
		return
	}
	defer rows.Close()

	items := make([]map[string]any, 0)
	for rows.Next() {
		var id int64
		var index int
		var content, createdAt string
		if err := rows.Scan(&id, &index, &content, &createdAt); err != nil {
			writeError(w, http.StatusInternalServerError, "读取文档分块失败")
			return
		}
		items = append(items, map[string]any{
			"id": id, "chunk_index": index, "content": content,
			"created_at": createdAt, "character_count": utf8.RuneCountInString(content),
		})
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "读取文档分块失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "success", "data": items})
}

func (h *Handler) UpdateLocalKnowledgeChunk(w http.ResponseWriter, r *http.Request) {
	payload, err := readJSONMap(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "请求参数错误")
		return
	}
	chunkID := intValue(payload["chunk_id"], 0)
	if chunkID < 1 {
		writeError(w, http.StatusBadRequest, "缺少或无效的分块 ID")
		return
	}
	content, validationError := validateKnowledgeChunkContent(stringValue(payload["content"]))
	if validationError != "" {
		writeError(w, http.StatusBadRequest, validationError)
		return
	}

	var baseID, documentID string
	if err := h.DB.QueryRowContext(r.Context(),
		"SELECT base_id,document_id FROM ai_knowledge_chunk WHERE id=?", chunkID,
	).Scan(&baseID, &documentID); errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "分块不存在")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "读取分块归属失败")
		return
	}
	if !h.requireKnowledgeBaseAccess(w, r, baseID) {
		return
	}

	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "保存分块失败")
		return
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(r.Context(),
		"UPDATE ai_knowledge_chunk SET content=? WHERE id=? AND document_id=?",
		content, chunkID, documentID,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "保存分块失败")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		writeError(w, http.StatusNotFound, "分块不存在")
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(r.Context(),
		"UPDATE ai_knowledge_document SET updated_at=? WHERE id=?",
		now, documentID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "更新文档时间失败")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "保存分块失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"code": 0, "msg": "updated",
		"data": map[string]any{"id": chunkID, "character_count": utf8.RuneCountInString(content)},
	})
}

func (h *Handler) CreateLocalKnowledgeChunk(w http.ResponseWriter, r *http.Request) {
	payload, err := readJSONMap(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "请求参数错误")
		return
	}
	documentID := strings.TrimSpace(stringValue(payload["document_id"]))
	if documentID == "" {
		writeError(w, http.StatusBadRequest, "缺少文档 ID")
		return
	}
	content, validationError := validateKnowledgeChunkContent(stringValue(payload["content"]))
	if validationError != "" {
		writeError(w, http.StatusBadRequest, validationError)
		return
	}

	var baseID string
	if err := h.DB.QueryRowContext(r.Context(),
		"SELECT base_id FROM ai_knowledge_document WHERE id=?", documentID,
	).Scan(&baseID); errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "文档不存在")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "读取文档归属失败")
		return
	}
	if !h.requireKnowledgeBaseAccess(w, r, baseID) {
		return
	}

	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "添加分块失败")
		return
	}
	defer tx.Rollback()
	var nextIndex int
	if err := tx.QueryRowContext(r.Context(),
		"SELECT COALESCE(MAX(chunk_index),-1)+1 FROM ai_knowledge_chunk WHERE document_id=?",
		documentID,
	).Scan(&nextIndex); err != nil {
		writeError(w, http.StatusInternalServerError, "计算分块序号失败")
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := tx.ExecContext(r.Context(),
		"INSERT INTO ai_knowledge_chunk (base_id,document_id,chunk_index,content,created_at) VALUES (?,?,?,?,?)",
		baseID, documentID, nextIndex, content, now,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "添加分块失败")
		return
	}
	chunkID, err := result.LastInsertId()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取新分块 ID 失败")
		return
	}
	if _, err := tx.ExecContext(r.Context(),
		"UPDATE ai_knowledge_document SET chunk_count=(SELECT COUNT(*) FROM ai_knowledge_chunk WHERE document_id=?),updated_at=? WHERE id=?",
		documentID, now, documentID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "更新文档分块数量失败")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "添加分块失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"code": 0, "msg": "created",
		"data": map[string]any{
			"id": chunkID, "chunk_index": nextIndex,
			"character_count": utf8.RuneCountInString(content),
		},
	})
}
