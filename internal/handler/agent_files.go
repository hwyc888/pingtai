package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"mime/multipart"
	"path/filepath"
	"strings"
)

func requestChatFiles(r *http.Request) []*multipart.FileHeader {
	if r == nil || r.MultipartForm == nil {
		return nil
	}
	files := r.MultipartForm.File["files"]
	if len(files) == 0 {
		files = r.MultipartForm.File["file"]
	}
	return files
}

func isKnowledgeImageFilename(filename string) bool {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".jpg", ".jpeg", ".png":
		return true
	default:
		return false
	}
}

func (h *Handler) uploadCozeFile(ctx context.Context, file multipart.File, header *multipart.FileHeader) (string, error) {
	if file == nil || header == nil {
		return "", errors.New("上传文件为空")
	}
	content, err := io.ReadAll(io.LimitReader(file, h.Config.MaxUploadBytes+1))
	if err != nil {
		return "", err
	}
	if int64(len(content)) > h.Config.MaxUploadBytes {
		return "", errors.New("文件超过大小限制")
	}
	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(header.Filename)))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	body, status, err := h.Coze.UploadFile(ctx, header.Filename, contentType, content)
	if err != nil {
		return "", err
	}
	if status < 200 || status >= 300 {
		return "", errors.New("Coze 文件上传请求失败")
	}
	var result struct {
		Code int `json:"code"`
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
		Msg string `json:"msg"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}
	if result.Code != 0 || result.Data.ID == "" {
		if result.Msg == "" {
			result.Msg = "Coze 未返回文件ID"
		}
		return "", errors.New(result.Msg)
	}
	return result.Data.ID, nil
}
