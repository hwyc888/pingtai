package handler

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"pingtai/internal/util"
)

type platformFile struct {
	ID string
	StorageKey string
	OriginalName string
	ContentType string
	Extension string
	SizeBytes int64
	SHA256 string
	ExtractedText string
	ExtractionStatus string
	ExtractionError string
	OwnerUserID string
	OwnerUsername string
	OwnerProvider string
	AccessMode string
	CreatedAt string
	UpdatedAt string
}

var fileCenterExtensions = map[string]bool{
	".pdf": true, ".docx": true, ".doc": true,
	".xlsx": true, ".xls": true, ".pptx": true, ".ppt": true,
	".txt": true, ".md": true, ".csv": true, ".json": true,
	".html": true, ".htm": true,
	".png": true, ".jpg": true, ".jpeg": true, ".webp": true,
}

func (h *Handler) fileStorageDir() string {
	if h.Config == nil {
		return filepath.Join(os.TempDir(), "pingtai", "files")
	}
	dbPath := strings.TrimSpace(h.Config.DatabasePath)
	if dbPath == "" || dbPath == ":memory:" {
		return filepath.Join(os.TempDir(), "pingtai", "files")
	}
	if strings.HasPrefix(dbPath, "file:") {
		dbPath = strings.TrimPrefix(dbPath, "file:")
		if query := strings.IndexByte(dbPath, '?'); query >= 0 {
			dbPath = dbPath[:query]
		}
	}
	absolute, err := filepath.Abs(dbPath)
	if err != nil {
		return filepath.Join(filepath.Dir(dbPath), "files")
	}
	return filepath.Join(filepath.Dir(absolute), "files")
}

func (h *Handler) ensureFileStorageDir() (string, error) {
	directory := h.fileStorageDir()
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", fmt.Errorf("create file storage directory: %w", err)
	}
	if err := os.Chmod(directory, 0700); err != nil {
		return "", fmt.Errorf("secure file storage directory: %w", err)
	}
	return directory, nil
}

func cleanUploadedFilename(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	value = filepath.Base(value)
	var builder strings.Builder
	for _, character := range value {
		if unicode.IsControl(character) || character == '/' || character == '\\' {
			continue
		}
		builder.WriteRune(character)
	}
	value = strings.TrimSpace(builder.String())
	if value == "" || value == "." || value == ".." {
		return "未命名文件"
	}
	runes := []rune(value)
	if len(runes) > 180 {
		value = string(runes[:180])
	}
	return value
}

func allowedFileCenterExtension(extension string) bool {
	return fileCenterExtensions[strings.ToLower(extension)]
}

func validFileStorageKey(value string) bool {
	if value == "" || filepath.Base(value) != value || strings.ContainsAny(value, "/\\") {
		return false
	}
	if strings.HasPrefix(value, ".") || strings.Contains(value, "..") {
		return false
	}
	return true
}

func fileAccessAllowed(ownerUserID, accessMode, userID string, admin bool) bool {
	if admin {
		return true
	}
	if strings.TrimSpace(userID) == "" {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(ownerUserID), strings.TrimSpace(userID)) {
		return true
	}
	return accessMode == "shared"
}

func fileSearchTerms(query string) []string {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	fields := strings.Fields(query)
	if len(fields) == 0 {
		return []string{query}
	}
	if len(fields) > 8 {
		fields = fields[:8]
	}
	return fields
}

func fileSearchSnippet(content, query string, maxRunes int) string {
	runes := []rune(strings.TrimSpace(content))
	if len(runes) == 0 {
		return ""
	}
	if maxRunes < 40 {
		maxRunes = 160
	}
	start := 0
	lower := []rune(strings.ToLower(string(runes)))
	for _, term := range fileSearchTerms(query) {
		needle := []rune(strings.ToLower(term))
		if len(needle) == 0 {
			continue
		}
		for index := 0; index+len(needle) <= len(lower); index++ {
			matched := true
			for offset := range needle {
				if lower[index+offset] != needle[offset] {
					matched = false
					break
				}
			}
			if matched {
				start = index - maxRunes/3
				if start < 0 {
					start = 0
				}
				goto found
			}
		}
	}
found:
	end := start + maxRunes
	if end > len(runes) {
		end = len(runes)
		start = end - maxRunes
		if start < 0 {
			start = 0
		}
	}
	prefix, suffix := "", ""
	if start > 0 {
		prefix = "…"
	}
	if end < len(runes) {
		suffix = "…"
	}
	return prefix + string(runes[start:end]) + suffix
}

func writeStoredFile(directory, storageKey string, content []byte) error {
	if !validFileStorageKey(storageKey) {
		return errors.New("invalid file storage key")
	}
	temporary, err := os.CreateTemp(directory, ".upload-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	target := filepath.Join(directory, storageKey)
	if err := os.Rename(temporaryPath, target); err != nil {
		return err
	}
	return os.Chmod(target, 0600)
}

func (h *Handler) SearchFiles(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.identityForRequest(r)
	if !ok || identity.UserID == "" {
		writeError(w, http.StatusUnauthorized, "请先登录")
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(query)) > 200 {
		writeError(w, http.StatusBadRequest, "搜索内容不能超过 200 个字符")
		return
	}
	extension := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("type")))
	extension = strings.TrimPrefix(extension, ".")
	if extension != "" && extension != "all" && !allowedFileCenterExtension("."+extension) {
		writeError(w, http.StatusBadRequest, "不支持的文件类型筛选")
		return
	}
	page := intValue(r.URL.Query().Get("page"), 1)
	pageSize := intValue(r.URL.Query().Get("page_size"), 20)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	admin := h.isKnowledgeAdmin(r.Context(), identity)
	conditions := []string{"1=1"}
	args := make([]any, 0, 16)
	if !admin {
		conditions = append(conditions, "(owner_user_id=? OR access_mode='shared')")
		args = append(args, identity.UserID)
	}
	if extension != "" && extension != "all" {
		conditions = append(conditions, "extension=?")
		args = append(args, "."+extension)
	}
	ownerFilter := strings.TrimSpace(r.URL.Query().Get("owner"))
	if admin && ownerFilter != "" {
		conditions = append(conditions, "owner_username LIKE ?")
		args = append(args, "%"+ownerFilter+"%")
	}
	for _, term := range fileSearchTerms(query) {
		conditions = append(conditions, "(instr(lower(original_name),lower(?))>0 OR instr(lower(extracted_text),lower(?))>0)")
		args = append(args, term, term)
	}
	where := strings.Join(conditions, " AND ")
	var total int
	if err := h.DB.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM platform_file WHERE "+where, args...).Scan(&total); err != nil {
		writeError(w, http.StatusInternalServerError, "检索文件失败")
		return
	}
	queryArgs := append(append([]any(nil), args...), pageSize, (page-1)*pageSize)
	rows, err := h.DB.QueryContext(r.Context(),
		"SELECT id,original_name,content_type,extension,size_bytes,sha256,extraction_status,extraction_error,"+
			"owner_user_id,owner_username,owner_provider,access_mode,created_at,updated_at,extracted_text "+
			"FROM platform_file WHERE "+where+" ORDER BY created_at DESC,id DESC LIMIT ? OFFSET ?",
		queryArgs...,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取文件检索结果失败")
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var item platformFile
		if err := rows.Scan(
			&item.ID, &item.OriginalName, &item.ContentType, &item.Extension, &item.SizeBytes, &item.SHA256,
			&item.ExtractionStatus, &item.ExtractionError, &item.OwnerUserID, &item.OwnerUsername,
			&item.OwnerProvider, &item.AccessMode, &item.CreatedAt, &item.UpdatedAt, &item.ExtractedText,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "读取文件检索结果失败")
			return
		}
		matchType := "recent"
		if query != "" {
			matchType = "content"
			for _, term := range fileSearchTerms(query) {
				if strings.Contains(strings.ToLower(item.OriginalName), strings.ToLower(term)) {
					matchType = "filename"
					break
				}
			}
		}
		snippet := fileSearchSnippet(item.ExtractedText, query, 180)
		if snippet == "" && item.ExtractionStatus != "indexed" {
			snippet = "已保存原始文件；当前格式暂不支持全文内容提取，可通过文件名检索。"
		}
		items = append(items, map[string]any{
			"id": item.ID, "name": item.OriginalName, "content_type": item.ContentType,
			"extension": item.Extension, "size_bytes": item.SizeBytes, "sha256": item.SHA256,
			"extraction_status": item.ExtractionStatus, "extraction_error": item.ExtractionError,
			"owner_user_id": item.OwnerUserID, "owner_username": item.OwnerUsername,
			"owner_provider": item.OwnerProvider, "access_mode": item.AccessMode,
			"created_at": item.CreatedAt, "updated_at": item.UpdatedAt,
			"match_type": matchType, "snippet": snippet,
			"can_delete": admin || item.OwnerUserID == identity.UserID,
			"can_preview": previewableFileExtension(item.Extension),
		})
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "读取文件检索结果失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"code": 0,
		"data": map[string]any{"items": items, "total": total, "page": page, "page_size": pageSize},
	})
}

func (h *Handler) UploadFiles(w http.ResponseWriter, r *http.Request) {
	if !h.parseForm(w, r) {
		return
	}
	identity, ok := h.identityForRequest(r)
	if !ok || identity.UserID == "" {
		writeError(w, http.StatusUnauthorized, "请先登录")
		return
	}
	if r.MultipartForm == nil {
		writeError(w, http.StatusBadRequest, "没有上传文件")
		return
	}
	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		files = r.MultipartForm.File["file"]
	}
	if len(files) == 0 || len(files) > 20 {
		writeError(w, http.StatusBadRequest, "请选择 1-20 个文件")
		return
	}
	accessMode := strings.ToLower(strings.TrimSpace(r.FormValue("access_mode")))
	if accessMode != "shared" {
		accessMode = "private"
	}
	directory, err := h.ensureFileStorageDir()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "初始化文件存储目录失败")
		return
	}
	maxBytes := h.Config.MaxUploadBytes
	if maxBytes <= 0 {
		maxBytes = 100 << 20
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	items := make([]map[string]any, 0, len(files))
	failures := make([]map[string]string, 0)
	for _, header := range files {
		if header == nil {
			continue
		}
		filename := cleanUploadedFilename(header.Filename)
		extension := strings.ToLower(filepath.Ext(filename))
		if !allowedFileCenterExtension(extension) {
			failures = append(failures, map[string]string{"name": filename, "error": "暂不支持该文件类型"})
			continue
		}
		file, openErr := header.Open()
		if openErr != nil {
			failures = append(failures, map[string]string{"name": filename, "error": "读取上传文件失败"})
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(file, maxBytes+1))
		_ = file.Close()
		if readErr != nil {
			failures = append(failures, map[string]string{"name": filename, "error": "读取上传文件失败"})
			continue
		}
		if int64(len(data)) > maxBytes {
			failures = append(failures, map[string]string{"name": filename, "error": "文件超过平台上传大小限制"})
			continue
		}
		digestBytes := sha256.Sum256(data)
		digest := hex.EncodeToString(digestBytes[:])
		var duplicateID, duplicateName string
		duplicateErr := h.DB.QueryRowContext(r.Context(),
			"SELECT id,original_name FROM platform_file WHERE owner_user_id=? AND sha256=?",
			identity.UserID, digest,
		).Scan(&duplicateID, &duplicateName)
		if duplicateErr == nil {
			items = append(items, map[string]any{
				"id": duplicateID, "name": duplicateName, "size_bytes": len(data),
				"duplicate": true, "message": "相同文件已存在，已复用原文件记录",
			})
			continue
		}
		if !errors.Is(duplicateErr, sql.ErrNoRows) {
			failures = append(failures, map[string]string{"name": filename, "error": "检查重复文件失败"})
			continue
		}
		extractedText, extractionErr := extractKnowledgeText(r.Context(), filename, data)
		extractionStatus := "indexed"
		extractionMessage := ""
		if extractionErr != nil {
			extractedText = ""
			extractionStatus = "filename_only"
			extractionMessage = extractionErr.Error()
			if len([]rune(extractionMessage)) > 240 {
				extractionMessage = string([]rune(extractionMessage)[:240])
			}
		}
		id, idErr := util.NewID()
		if idErr != nil {
			failures = append(failures, map[string]string{"name": filename, "error": "生成文件 ID 失败"})
			continue
		}
		storageKey := id + extension
		if writeErr := writeStoredFile(directory, storageKey, data); writeErr != nil {
			failures = append(failures, map[string]string{"name": filename, "error": "保存原始文件失败"})
			continue
		}
		contentType := mime.TypeByExtension(extension)
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		_, insertErr := h.DB.ExecContext(r.Context(),
			"INSERT INTO platform_file (id,storage_key,original_name,content_type,extension,size_bytes,sha256,extracted_text,extraction_status,extraction_error,owner_user_id,owner_username,owner_provider,access_mode,created_at,updated_at) "+
				"VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(owner_user_id,sha256) DO NOTHING",
			id, storageKey, filename, contentType, extension, int64(len(data)), digest, extractedText,
			extractionStatus, extractionMessage, identity.UserID, identity.Username, identity.Provider, accessMode, now, now,
		)
		if insertErr != nil {
			_ = os.Remove(filepath.Join(directory, storageKey))
			failures = append(failures, map[string]string{"name": filename, "error": "保存文件索引失败"})
			continue
		}
		var storedID string
		if err := h.DB.QueryRowContext(r.Context(),
			"SELECT id FROM platform_file WHERE owner_user_id=? AND sha256=?",
			identity.UserID, digest,
		).Scan(&storedID); err != nil {
			_ = os.Remove(filepath.Join(directory, storageKey))
			failures = append(failures, map[string]string{"name": filename, "error": "确认文件保存状态失败"})
			continue
		}
		if storedID != id {
			_ = os.Remove(filepath.Join(directory, storageKey))
			items = append(items, map[string]any{
				"id": storedID, "name": filename, "size_bytes": len(data),
				"duplicate": true, "message": "相同文件已存在，已复用原文件记录",
			})
			continue
		}
		items = append(items, map[string]any{
			"id": id, "name": filename, "size_bytes": len(data),
			"extraction_status": extractionStatus, "extraction_error": extractionMessage,
			"access_mode": accessMode, "duplicate": false,
		})
	}
	if len(items) == 0 && len(failures) > 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"code": http.StatusBadRequest, "msg": "没有文件上传成功", "data": map[string]any{"failed": failures},
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"code": 0, "msg": "文件上传完成",
		"data": map[string]any{"items": items, "failed": failures},
	})
}

func (h *Handler) fileRecord(ctx context.Context, id string) (platformFile, error) {
	var item platformFile
	err := h.DB.QueryRowContext(ctx,
		"SELECT id,storage_key,original_name,content_type,extension,size_bytes,sha256,extraction_status,"+
			"owner_user_id,owner_username,owner_provider,access_mode,created_at,updated_at "+
			"FROM platform_file WHERE id=?",
		id,
	).Scan(
		&item.ID, &item.StorageKey, &item.OriginalName, &item.ContentType, &item.Extension,
		&item.SizeBytes, &item.SHA256, &item.ExtractionStatus, &item.OwnerUserID,
		&item.OwnerUsername, &item.OwnerProvider, &item.AccessMode, &item.CreatedAt, &item.UpdatedAt,
	)
	return item, err
}

func (h *Handler) serveStoredFile(w http.ResponseWriter, r *http.Request, preview bool) {
	identity, ok := h.identityForRequest(r)
	if !ok || identity.UserID == "" {
		writeError(w, http.StatusUnauthorized, "请先登录")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	item, err := h.fileRecord(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !fileAccessAllowed(item.OwnerUserID, item.AccessMode, identity.UserID, h.isKnowledgeAdmin(r.Context(), identity))) {
		writeError(w, http.StatusNotFound, "文件不存在或无权访问")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取文件信息失败")
		return
	}
	if !validFileStorageKey(item.StorageKey) {
		writeError(w, http.StatusInternalServerError, "文件存储信息无效")
		return
	}
	if preview && !previewableFileExtension(item.Extension) {
		writeError(w, http.StatusUnsupportedMediaType, "该文件类型暂不支持在线预览，请下载后打开")
		return
	}
	path := filepath.Join(h.fileStorageDir(), item.StorageKey)
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, "原始文件不存在，请联系管理员检查文件存储目录")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "打开原始文件失败")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取原始文件信息失败")
		return
	}
	contentType := item.ContentType
	if strings.EqualFold(item.Extension, ".html") || strings.EqualFold(item.Extension, ".htm") ||
		strings.EqualFold(item.Extension, ".md") || strings.EqualFold(item.Extension, ".csv") ||
		strings.EqualFold(item.Extension, ".json") || strings.EqualFold(item.Extension, ".txt") {
		contentType = "text/plain; charset=utf-8"
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	disposition := "attachment"
	if preview {
		disposition = "inline"
	}
	header := mime.FormatMediaType(disposition, map[string]string{"filename": item.OriginalName})
	if header == "" {
		header = disposition
	}
	w.Header().Set("Content-Disposition", header)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	http.ServeContent(w, r, item.OriginalName, info.ModTime(), file)
}

func previewableFileExtension(extension string) bool {
	switch strings.ToLower(extension) {
	case ".pdf", ".png", ".jpg", ".jpeg", ".webp", ".txt", ".md", ".csv", ".json", ".html", ".htm":
		return true
	default:
		return false
	}
}

func (h *Handler) DownloadFile(w http.ResponseWriter, r *http.Request) {
	h.serveStoredFile(w, r, false)
}

func (h *Handler) PreviewFile(w http.ResponseWriter, r *http.Request) {
	h.serveStoredFile(w, r, true)
}

func (h *Handler) DeleteFile(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.identityForRequest(r)
	if !ok || identity.UserID == "" {
		writeError(w, http.StatusUnauthorized, "请先登录")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	// Resolve administrator status before opening a transaction. The SQLite pool
	// intentionally uses one connection, so querying the user table while a
	// transaction owns that connection would deadlock.
	admin := h.isKnowledgeAdmin(r.Context(), identity)
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "删除文件失败")
		return
	}
	defer tx.Rollback()
	var storageKey, ownerUserID string
	err = tx.QueryRowContext(r.Context(),
		"SELECT storage_key,owner_user_id FROM platform_file WHERE id=?", id,
	).Scan(&storageKey, &ownerUserID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "文件不存在")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取文件信息失败")
		return
	}
	if !admin && ownerUserID != identity.UserID {
		writeError(w, http.StatusForbidden, "只有文件上传者或管理员可以删除文件")
		return
	}
	if !validFileStorageKey(storageKey) {
		writeError(w, http.StatusInternalServerError, "文件存储信息无效")
		return
	}
	sourcePath := filepath.Join(h.fileStorageDir(), storageKey)
	trashPath := filepath.Join(h.fileStorageDir(), ".delete-"+id)
	renamed := false
	if err := os.Rename(sourcePath, trashPath); err == nil {
		renamed = true
	} else if !errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusInternalServerError, "暂时无法删除原始文件")
		return
	}
	if _, err := tx.ExecContext(r.Context(), "DELETE FROM platform_file WHERE id=?", id); err != nil {
		if renamed {
			_ = os.Rename(trashPath, sourcePath)
		}
		writeError(w, http.StatusInternalServerError, "删除文件索引失败")
		return
	}
	if err := tx.Commit(); err != nil {
		if renamed {
			_ = os.Rename(trashPath, sourcePath)
		}
		writeError(w, http.StatusInternalServerError, "提交文件删除失败")
		return
	}
	if renamed {
		_ = os.Remove(trashPath)
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "文件已删除"})
}
