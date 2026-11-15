package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"pingtai/internal/service/coze"
)

func (h *Handler) ListKnowledgeVersions(w http.ResponseWriter, r *http.Request) {
	datasetID := strings.TrimSpace(r.URL.Query().Get("dataset_id"))
	if datasetID == "" {
		writeError(w, http.StatusBadRequest, "缺少知识库 ID")
		return
	}
	rows, err := h.DB.QueryContext(r.Context(), `
		SELECT id, version_number, title, description, status, snapshot_json,
		       document_count, created_by, created_at, published_at
		FROM ai_knowledge_version
		WHERE dataset_id=?
		ORDER BY version_number DESC
	`, datasetID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取知识库版本失败")
		return
	}
	defer rows.Close()

	items := make([]map[string]any, 0)
	for rows.Next() {
		var id int64
		var versionNumber, documentCount int
		var title, description, status, snapshotJSON, createdBy, createdAt, publishedAt string
		if err := rows.Scan(&id, &versionNumber, &title, &description, &status, &snapshotJSON,
			&documentCount, &createdBy, &createdAt, &publishedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "读取知识库版本失败")
			return
		}
		var snapshot map[string]any
		_ = json.Unmarshal([]byte(snapshotJSON), &snapshot)
		documents, _ := snapshot["documents"].([]any)
		if documents == nil {
			documents = []any{}
		}
		items = append(items, map[string]any{
			"id": id,
			"dataset_id": datasetID,
			"version_number": versionNumber,
			"title": title,
			"description": description,
			"status": status,
			"document_count": documentCount,
			"created_by": createdBy,
			"created_at": createdAt,
			"published_at": publishedAt,
			"documents": documents,
			"captured_at": snapshot["captured_at"],
		})
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "读取知识库版本失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": items})
}

func (h *Handler) PublishKnowledgeVersion(w http.ResponseWriter, r *http.Request) {
	payload, err := readJSONMap(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "请求参数错误")
		return
	}
	datasetID := strings.TrimSpace(stringValue(payload["dataset_id"]))
	title := strings.TrimSpace(stringValue(payload["title"]))
	description := strings.TrimSpace(stringValue(payload["description"]))
	if datasetID == "" || len(datasetID) > 128 {
		writeError(w, http.StatusBadRequest, "知识库 ID 无效")
		return
	}
	if len([]rune(title)) > 80 || len([]rune(description)) > 500 {
		writeError(w, http.StatusBadRequest, "版本名称最多 80 个字符，说明最多 500 个字符")
		return
	}

	documents, err := h.listKnowledgeDocuments(r.Context(), datasetID)
	if err != nil {
		writeError(w, http.StatusBadGateway, "读取 Coze 知识库文档失败："+err.Error())
		return
	}
	if len(documents) == 0 {
		writeError(w, http.StatusBadRequest, "知识库暂无文档，暂不能发布版本")
		return
	}
	for _, document := range documents {
		if intValue(document["status"], -1) != 1 {
			writeError(w, http.StatusConflict, "仍有文档处于处理中或处理失败，请等待全部文档处理完成后再发布")
			return
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	snapshot, err := json.Marshal(map[string]any{
		"dataset_id": datasetID,
		"captured_at": now,
		"document_count": len(documents),
		"documents": documents,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "生成版本快照失败")
		return
	}

	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "创建版本事务失败")
		return
	}
	defer tx.Rollback()

	var versionNumber int
	if err := tx.QueryRowContext(r.Context(),
		"SELECT COALESCE(MAX(version_number),0)+1 FROM ai_knowledge_version WHERE dataset_id=?", datasetID,
	).Scan(&versionNumber); err != nil {
		writeError(w, http.StatusInternalServerError, "生成版本号失败")
		return
	}
	if title == "" {
		title = fmt.Sprintf("版本 %d", versionNumber)
	}
	identity, _ := currentIdentity(r)
	createdBy := strings.TrimSpace(identity.Username)
	if createdBy == "" {
		createdBy = "administrator"
	}
	result, err := tx.ExecContext(r.Context(), `
		INSERT INTO ai_knowledge_version
		(dataset_id, version_number, title, description, status, snapshot_json, document_count, created_by, created_at, published_at)
		VALUES (?, ?, ?, ?, 'published', ?, ?, ?, ?, ?)
	`, datasetID, versionNumber, title, description, string(snapshot), len(documents), createdBy, now, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "保存知识库版本失败")
		return
	}
	id, err := result.LastInsertId()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取版本 ID 失败")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "发布知识库版本失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"code": 0,
		"msg": "published",
		"data": map[string]any{
			"id": id,
			"dataset_id": datasetID,
			"version_number": versionNumber,
			"title": title,
			"description": description,
			"status": "published",
			"document_count": len(documents),
			"created_by": createdBy,
			"created_at": now,
			"published_at": now,
			"documents": documents,
		},
	})
}

func (h *Handler) listKnowledgeDocuments(ctx context.Context, datasetID string) ([]map[string]any, error) {
	if h.Coze == nil || !h.Coze.Configured() {
		return nil, errors.New("Coze 知识库服务未配置")
	}
	documents := make([]map[string]any, 0)
	headers := make(http.Header)
	headers.Set("Agw-Js-Conv", "str")
	const pageSize = 50
	for page := 1; page <= 100; page++ {
		resp, err := h.Coze.DoJSON(ctx, http.MethodPost, "/open_api/knowledge/document/list", nil,
			map[string]any{"dataset_id": datasetID, "page": page, "size": pageSize}, headers)
		if err != nil {
			return nil, err
		}
		body, status, err := coze.ReadResponse(resp)
		if err != nil {
			return nil, err
		}
		if status < 200 || status >= 300 {
			return nil, fmt.Errorf("Coze HTTP %d", status)
		}
		var result struct {
			Code          int              `json:"code"`
			Msg           string           `json:"msg"`
			Total         int              `json:"total"`
			DocumentInfos []map[string]any `json:"document_infos"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("Coze 返回格式无效: %w", err)
		}
		if result.Code != 0 {
			return nil, fmt.Errorf("Coze 错误 %d: %s", result.Code, result.Msg)
		}
		documents = append(documents, result.DocumentInfos...)
		if len(result.DocumentInfos) == 0 || result.Total == 0 || len(documents) >= result.Total {
			return documents, nil
		}
	}
	return nil, errors.New("知识库文档数量超过单次版本快照限制（5000）")
}
