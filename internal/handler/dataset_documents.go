package handler

import (
	"encoding/base64"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
)

func (h *Handler) KnowledgeFileList(w http.ResponseWriter, r *http.Request) {
	payload, err := readJSONMap(r)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "请求参数错误"})
		return
	}
	datasetID := strings.TrimSpace(stringValue(payload["dataset_id"]))
	if datasetID == "" {
		writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "缺少知识库ID参数"})
		return
	}
	data := map[string]any{
		"dataset_id": datasetID,
		"page":       defaultString(stringValue(payload["page_num"]), "1"),
		"size":       defaultString(stringValue(payload["page_size"]), "10"),
	}
	headers := make(http.Header)
	headers.Set("Agw-Js-Conv", "str")
	h.callCozeJSON(w, r, http.MethodPost, "/open_api/knowledge/document/list", nil, data, headers)
}

func (h *Handler) CreateKnowledgeFile(w http.ResponseWriter, r *http.Request) {
	if !h.parseForm(w, r) {
		return
	}
	datasetID := strings.TrimSpace(r.FormValue("dataset_id"))
	if datasetID == "" {
		writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "缺少知识库ID参数"})
		return
	}
	formatType, err := strconv.Atoi(defaultString(r.FormValue("format_type"), "0"))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "format_type 参数错误"})
		return
	}

	documentBases := make([]map[string]any, 0)
	var chunkStrategy map[string]any
	switch formatType {
	case 0:
		chunkStrategy = map[string]any{
			"separator": "\n\n", "max_tokens": 800,
			"remove_extra_spaces": false, "remove_urls_emails": false, "chunk_type": 0,
		}
		switch r.FormValue("source") {
		case "file":
			files := r.MultipartForm.File["files"]
			if len(files) == 0 {
				writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "没有上传文件"})
				return
			}
			documentBases, err = h.documentBasesFromFiles(files, map[string]bool{
				".txt": true, ".pdf": true, ".doc": true, ".docx": true,
			})
		case "url":
			webURL := strings.TrimSpace(r.FormValue("web_url"))
			if webURL == "" {
				writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "缺少网页链接"})
				return
			}
			updateType, _ := strconv.Atoi(defaultString(r.FormValue("update_type"), "0"))
			updateInterval, _ := strconv.Atoi(defaultString(r.FormValue("update_interval"), "24"))
			documentBases = append(documentBases, map[string]any{
				"name":        r.FormValue("name"),
				"source_info": map[string]any{"web_url": webURL, "document_source": 1},
				"update_rule": map[string]any{"update_type": updateType, "update_interval": updateInterval},
			})
		default:
			writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "source 必须为 file 或 url"})
			return
		}
	case 2:
		chunkStrategy = map[string]any{"caption_type": 0}
		files := r.MultipartForm.File["files"]
		if len(files) == 0 {
			files = r.MultipartForm.File["file"]
		}
		documentBases, err = h.documentBasesFromFiles(files, map[string]bool{
			".jpg": true, ".jpeg": true, ".png": true, ".webp": true,
		})
	default:
		writeJSON(w, http.StatusOK, map[string]any{"code": 400, "msg": "不支持的知识库格式类型"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"code": 400, "msg": err.Error()})
		return
	}
	if len(documentBases) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "没有有效的文件内容"})
		return
	}
	data := map[string]any{
		"dataset_id": datasetID, "format_type": formatType,
		"document_bases": documentBases, "chunk_strategy": chunkStrategy,
	}
	headers := make(http.Header)
	headers.Set("Agw-Js-Conv", "str")
	h.callCozeJSON(w, r, http.MethodPost, "/open_api/knowledge/document/create", nil, data, headers)
}

func (h *Handler) documentBasesFromFiles(files []*multipart.FileHeader, allowed map[string]bool) ([]map[string]any, error) {
	result := make([]map[string]any, 0, len(files))
	for _, header := range files {
		if header == nil || header.Filename == "" {
			continue
		}
		extension := strings.ToLower(filepath.Ext(header.Filename))
		if !allowed[extension] {
			return nil, errors.New("不支持的文件类型: " + extension)
		}
		file, err := header.Open()
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(io.LimitReader(file, h.Config.MaxUploadBytes+1))
		_ = file.Close()
		if readErr != nil {
			return nil, readErr
		}
		if int64(len(data)) > h.Config.MaxUploadBytes {
			return nil, errors.New("文件超过大小限制: " + header.Filename)
		}
		fileType := strings.TrimPrefix(extension, ".")
		result = append(result, map[string]any{
			"source_info": map[string]any{
				"file_base64": base64.StdEncoding.EncodeToString(data),
				"file_type":   fileType,
			},
			"name": header.Filename,
		})
	}
	return result, nil
}

func (h *Handler) DeleteKnowledgeFile(w http.ResponseWriter, r *http.Request) {
	payload, err := readJSONMap(r)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "请求参数错误"})
		return
	}
	documentIDs := payload["document_ids"]
	if documentIDs == nil {
		writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "缺少文件ID参数"})
		return
	}
	headers := make(http.Header)
	headers.Set("Agw-Js-Conv", "str")
	h.callCozeJSON(w, r, http.MethodPost, "/open_api/knowledge/document/delete", nil,
		map[string]any{"document_ids": documentIDs}, headers)
}

func (h *Handler) GetKnowledgeFileProcess(w http.ResponseWriter, r *http.Request) {
	payload, err := readJSONMap(r)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "请求参数错误"})
		return
	}
	datasetID := strings.TrimSpace(stringValue(payload["dataset_id"]))
	documentIDs := payload["document_ids"]
	if datasetID == "" || documentIDs == nil {
		writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "缺少知识库ID或文件ID参数"})
		return
	}
	headers := make(http.Header)
	headers.Set("Agw-Js-Conv", "str")
	h.callCozeJSON(w, r, http.MethodPost, "/open_api/knowledge/document/process", nil,
		map[string]any{"dataset_id": datasetID, "document_ids": documentIDs}, headers)
}

func (h *Handler) UpdateKnowledgeFile(w http.ResponseWriter, r *http.Request) {
	payload, err := readJSONMap(r)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "请求参数错误"})
		return
	}
	documentID := strings.TrimSpace(stringValue(payload["document_id"]))
	if documentID == "" {
		writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "缺少文件ID参数"})
		return
	}
	data := map[string]any{"document_id": documentID}
	if name := stringValue(payload["document_name"]); name != "" {
		data["document_name"] = name
	}
	if payload["update_type"] != nil {
		updateType := intValue(payload["update_type"], 0)
		updateInterval := intValue(payload["update_interval"], 24)
		data["update_rule"] = map[string]any{
			"update_type": updateType, "update_interval": updateInterval,
		}
	}
	headers := make(http.Header)
	headers.Set("Agw-Js-Conv", "str")
	h.callCozeJSON(w, r, http.MethodPost, "/open_api/knowledge/document/update", nil, data, headers)
}
