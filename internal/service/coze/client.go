package coze

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"sync"
	"time"

	"pingtai/internal/config"
)

type Client struct {
	cfg         config.Config
	client      *http.Client
	mu          sync.Mutex
	cachedToken string
	tokenExpiry time.Time
}

func NewClient(cfg config.Config) *Client {
	cfg.CozeBaseURL = strings.TrimRight(cfg.CozeBaseURL, "/")
	return &Client{
		cfg:    cfg,
		client: &http.Client{Timeout: 180 * time.Second},
	}
}

func (c *Client) Configured() bool {
	return c.cfg.CozeConfigured()
}

func (c *Client) Do(ctx context.Context, method, path string, query url.Values, body []byte, headers http.Header) (*http.Response, error) {
	if !c.Configured() {
		return nil, errors.New("Coze is not configured; set COZE_ACCESS_TOKEN or the OAuth JWT credentials")
	}
	token, err := c.getToken(ctx)
	if err != nil {
		return nil, err
	}
	target, err := url.Parse(c.cfg.CozeBaseURL + "/" + strings.TrimLeft(path, "/"))
	if err != nil {
		return nil, err
	}
	if query != nil {
		target.RawQuery = query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", token)
	if body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	return c.client.Do(req)
}

func (c *Client) DoJSON(ctx context.Context, method, path string, query url.Values, payload any, headers http.Header) (*http.Response, error) {
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return nil, err
		}
	}
	if headers == nil {
		headers = make(http.Header)
	}
	if body != nil && headers.Get("Content-Type") == "" {
		headers.Set("Content-Type", "application/json")
	}
	return c.Do(ctx, method, path, query, body, headers)
}

func (c *Client) UploadFile(ctx context.Context, filename, contentType string, data []byte) ([]byte, int, error) {
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	partHeader := make(textproto.MIMEHeader)
	partHeader.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": "file", "filename": filename}))
	partHeader.Set("Content-Type", contentType)
	part, err := writer.CreatePart(partHeader)
	if err != nil {
		return nil, 0, err
	}
	if _, err := part.Write(data); err != nil {
		return nil, 0, err
	}
	if err := writer.Close(); err != nil {
		return nil, 0, err
	}
	headers := make(http.Header)
	headers.Set("Content-Type", writer.FormDataContentType())
	resp, err := c.Do(ctx, http.MethodPost, "/v1/files/upload", nil, buffer.Bytes(), headers)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	return body, resp.StatusCode, err
}

func ReadResponse(resp *http.Response) ([]byte, int, error) {
	if resp == nil {
		return nil, 0, errors.New("empty upstream response")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	return body, resp.StatusCode, err
}

func JSONError(status int, err error) []byte {
	payload, _ := json.Marshal(map[string]any{"code": status, "msg": err.Error(), "error": true})
	return payload
}

func ErrorStatus(err error) int {
	if err == nil {
		return 0
	}
	return http.StatusBadGateway
}

func ErrorMessage(status int, body []byte) string {
	if len(body) == 0 {
		return fmt.Sprintf("upstream returned status %d", status)
	}
	return string(body)
}
