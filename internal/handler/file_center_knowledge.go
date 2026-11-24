package handler

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pingtai/internal/middleware"
	"pingtai/internal/util"
)

func (h *Handler) persistKnowledgeOriginalFileTx(
	ctx context.Context,
	tx *sql.Tx,
	directory string,
	identity middleware.Identity,
	filename string,
	contentType string,
	data []byte,
	extractedText string,
	now string,
) (string, string, error) {
	digestBytes := sha256.Sum256(data)
	digest := hex.EncodeToString(digestBytes[:])
	var fileID, storageKey string
	err := tx.QueryRowContext(ctx,
		"SELECT id,storage_key FROM platform_file WHERE owner_user_id=? AND sha256=?",
		identity.UserID, digest,
	).Scan(&fileID, &storageKey)
	if err == nil {
		if !validFileStorageKey(storageKey) {
			return "", "", errors.New("existing file storage key is invalid")
		}
		path := filepath.Join(directory, storageKey)
		if _, statErr := os.Stat(path); errors.Is(statErr, os.ErrNotExist) {
			if writeErr := writeStoredFile(directory, storageKey, data); writeErr != nil {
				return "", "", fmt.Errorf("restore original file: %w", writeErr)
			}
		} else if statErr != nil {
			return "", "", statErr
		}
		_, updateErr := tx.ExecContext(ctx,
			"UPDATE platform_file SET extracted_text=CASE WHEN extracted_text='' THEN ? ELSE extracted_text END,"+
				"extraction_status=CASE WHEN extraction_status<>'indexed' AND ?<>'' THEN 'indexed' ELSE extraction_status END,"+
				"extraction_error=CASE WHEN ?<>'' THEN '' ELSE extraction_error END,updated_at=? WHERE id=?",
			extractedText, extractedText, extractedText, now, fileID,
		)
		if updateErr != nil {
			return "", "", updateErr
		}
		return fileID, "", nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", "", err
	}

	fileID, err = util.NewID()
	if err != nil {
		return "", "", err
	}
	extension := strings.ToLower(filepath.Ext(filename))
	storageKey = fileID + extension
	if !validFileStorageKey(storageKey) {
		return "", "", errors.New("generated file storage key is invalid")
	}
	path := filepath.Join(directory, storageKey)
	if err := writeStoredFile(directory, storageKey, data); err != nil {
		return "", "", err
	}
	if contentType == "" {
		contentType = mime.TypeByExtension(extension)
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	_, err = tx.ExecContext(ctx,
		"INSERT INTO platform_file (id,storage_key,original_name,content_type,extension,size_bytes,sha256,extracted_text,"+
			"extraction_status,extraction_error,owner_user_id,owner_username,owner_provider,access_mode,created_at,updated_at) "+
			"VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,'private',?,?) ON CONFLICT(owner_user_id,sha256) DO NOTHING",
		fileID, storageKey, filename, contentType, extension, int64(len(data)), digest, extractedText,
		"indexed", "", identity.UserID, identity.Username, identity.Provider, now, now,
	)
	if err != nil {
		_ = os.Remove(path)
		return "", "", err
	}
	var storedID string
	if err := tx.QueryRowContext(ctx,
		"SELECT id FROM platform_file WHERE owner_user_id=? AND sha256=?",
		identity.UserID, digest,
	).Scan(&storedID); err != nil {
		_ = os.Remove(path)
		return "", "", err
	}
	if storedID != fileID {
		_ = os.Remove(path)
		return storedID, "", nil
	}
	return fileID, path, nil
}

func (h *Handler) canAccessStoredFile(ctx context.Context, identity middleware.Identity, item platformFile) (bool, error) {
	admin := h.isKnowledgeAdmin(ctx, identity)
	if fileAccessAllowed(item.OwnerUserID, item.AccessMode, identity.UserID, admin) {
		return true, nil
	}
	rows, err := h.DB.QueryContext(ctx,
		"SELECT base_id FROM ai_knowledge_document_file WHERE file_id=?",
		item.ID,
	)
	if err != nil {
		return false, err
	}
	baseIDs := make([]string, 0)
	for rows.Next() {
		var baseID string
		if err := rows.Scan(&baseID); err != nil {
			rows.Close()
			return false, err
		}
		baseIDs = append(baseIDs, baseID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return false, err
	}
	if err := rows.Close(); err != nil {
		return false, err
	}
	for _, baseID := range baseIDs {
		canUse, _, err := h.knowledgeAccess(ctx, identity, baseID)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return false, err
		}
		if canUse {
			return true, nil
		}
	}
	return false, nil
}

func (h *Handler) AttachFileToKnowledgeBase(w http.ResponseWriter, r *http.Request) {
	payload, err := readJSONMap(r)
	if err != nil {
		writeError(w, 400, "请求参数格式错误")
		return
	}
	baseID := strings.TrimSpace(stringValue(payload["base_id"]))
	fileID := strings.TrimSpace(stringValue(payload["file_id"]))
	if baseID == "" || fileID == "" {
		writeError(w, 400, "请选择知识库和文件")
		return
	}
	if !h.requireKnowledgeBaseAccess(w, r, baseID) {
		return
	}
	identity, ok := h.identityForRequest(r)
	if !ok || identity.UserID == "" {
		writeError(w, 401, "请先登录")
		return
	}
	item, err := h.fileRecord(r.Context(), fileID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "文件不存在")
		return
	}
	if err != nil {
		writeError(w, 500, "读取文件信息失败")
		return
	}
	canAccess, err := h.canAccessStoredFile(r.Context(), identity, item)
	if err != nil {
		writeError(w, 500, "验证文件权限失败")
		return
	}
	if !canAccess {
		writeError(w, 404, "文件不存在或无权访问")
		return
	}
	var existingDocumentID string
	if err := h.DB.QueryRowContext(r.Context(),
		"SELECT document_id FROM ai_knowledge_document_file WHERE base_id=? AND file_id=?",
		baseID, fileID,
	).Scan(&existingDocumentID); err == nil {
		writeJSON(w, 200, map[string]any{
			"code": 0, "msg": "文件已经关联到该知识库",
			"data": map[string]any{"document_id": existingDocumentID, "file_id": fileID, "duplicate": true},
		})
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		writeError(w, 500, "检查知识库文件关联失败")
		return
	}
	if !validFileStorageKey(item.StorageKey) {
		writeError(w, 500, "文件存储信息无效")
		return
	}
	path := filepath.Join(h.fileStorageDir(), item.StorageKey)
	fileBytes, err := os.ReadFile(path)
	if err != nil {
		writeError(w, 404, "原始文件不存在，请联系管理员检查文件存储目录")
		return
	}
	maxBytes := int64(100 << 20)
	if h.Config != nil && h.Config.MaxUploadBytes > 0 {
		maxBytes = h.Config.MaxUploadBytes
	}
	if int64(len(fileBytes)) > maxBytes {
		writeError(w, 413, "文件超过平台索引大小限制")
		return
	}
	content := strings.TrimSpace(item.ExtractedText)
	if content == "" {
		content, err = extractKnowledgeText(r.Context(), item.OriginalName, fileBytes)
		if err != nil {
			writeError(w, 400, item.OriginalName+": "+err.Error())
			return
		}
	}
	chunks := splitKnowledgeText(content)
	if len(chunks) == 0 {
		writeError(w, 400, "文件没有可索引的文本内容")
		return
	}
	documentID, err := util.NewID()
	if err != nil {
		writeError(w, 500, "生成知识库文档 ID 失败")
		return
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, 500, "关联知识文件失败")
		return
	}
	defer tx.Rollback()
	if err := tx.QueryRowContext(r.Context(),
		"SELECT document_id FROM ai_knowledge_document_file WHERE base_id=? AND file_id=?",
		baseID, fileID,
	).Scan(&existingDocumentID); err == nil {
		writeJSON(w, 200, map[string]any{
			"code": 0, "msg": "文件已经关联到该知识库",
			"data": map[string]any{"document_id": existingDocumentID, "file_id": fileID, "duplicate": true},
		})
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		writeError(w, 500, "检查知识库文件关联失败")
		return
	}
	if _, err := tx.ExecContext(r.Context(),
		"INSERT INTO ai_knowledge_document (id,base_id,filename,content_type,size_bytes,content_text,chunk_count,created_by,created_at,updated_at) "+
			"VALUES (?,?,?,?,?,?,?,?,?,?)",
		documentID, baseID, item.OriginalName, item.ContentType, item.SizeBytes, content, len(chunks), identity.Username, now, now,
	); err != nil {
		writeError(w, 500, "保存知识库文档失败")
		return
	}
	if _, err := tx.ExecContext(r.Context(),
		"INSERT INTO ai_knowledge_document_file (document_id,base_id,file_id,created_at) VALUES (?,?,?,?)",
		documentID, baseID, fileID, now,
	); err != nil {
		writeError(w, 500, "关联原始文件失败")
		return
	}
	for index, chunk := range chunks {
		if _, err := tx.ExecContext(r.Context(),
			"INSERT INTO ai_knowledge_chunk (base_id,document_id,chunk_index,content,created_at) VALUES (?,?,?,?,?)",
			baseID, documentID, index, chunk, now,
		); err != nil {
			writeError(w, 500, "建立知识库索引失败")
			return
		}
	}
	if _, err := tx.ExecContext(r.Context(), "UPDATE ai_knowledge_base SET updated_at=? WHERE id=?", now, baseID); err != nil {
		writeError(w, 500, "更新知识库时间失败")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, 500, "提交知识库文件关联失败")
		return
	}
	writeJSON(w, 200, map[string]any{
		"code": 0, "msg": "文件已加入知识库",
		"data": map[string]any{"document_id": documentID, "file_id": fileID, "filename": item.OriginalName, "chunk_count": len(chunks)},
	})
}
