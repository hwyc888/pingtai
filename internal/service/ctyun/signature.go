package ctyun

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"pingtai/internal/config"
)

func GenerateAuthorization(endpoint string, appID, appKey, region, prefix string, body []byte, now time.Time, ttlSeconds int) (string, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	if prefix == "" {
		prefix = "teleai-cloud-auth-v1"
	}
	if ttlSeconds <= 0 {
		ttlSeconds = 1800
	}
	if region == "" {
		region = "QG"
	}
	timestamp := now.Unix()
	basic := strings.Join([]string{
		prefix, appID, region, formatInt(timestamp), formatInt(int64(ttlSeconds)),
	}, "/")
	signedHeaders := "x-app-id"
	canonicalHeaders := "x-app-id:" + encodeComponent(appID)
	canonicalQuery := canonicalQueryString(parsed.Query())
	canonicalURI := parsed.EscapedPath()
	if canonicalURI == "" {
		canonicalURI = "/"
	}
	payloadHash := sha256.Sum256(body)
	canonicalRequest := strings.Join([]string{
		"POST",
		canonicalURI,
		canonicalQuery,
		canonicalHeaders,
		signedHeaders,
		hex.EncodeToString(payloadHash[:]),
	}, "\n")

	signingKey := hmacHex([]byte(appKey), basic)
	signature := hmacHex([]byte(signingKey), canonicalRequest)
	return basic + "/" + signedHeaders + "/" + signature, nil
}

func BuildSignatureHeaders(endpoint string, cfg config.Config, body []byte, now time.Time) (map[string]string, error) {
	authorization, err := GenerateAuthorization(
		endpoint, cfg.CTYunAppID, cfg.CTYunAppKey, cfg.CTYunRegion,
		cfg.CTYunSignPrefix, body, now, 1800,
	)
	if err != nil {
		return nil, err
	}
	headers := map[string]string{
		"X-APP-ID":      cfg.CTYunAppID,
		"Authorization": authorization,
	}
	if cfg.CTYunOrderNum != "" {
		headers["Order-Num"] = cfg.CTYunOrderNum
	}
	return headers, nil
}

func canonicalQueryString(values url.Values) string {
	items := make([]string, 0)
	for key, entries := range values {
		if len(entries) == 0 {
			items = append(items, encodeComponent(key)+"=")
			continue
		}
		for _, value := range entries {
			items = append(items, encodeComponent(key)+"="+encodeComponent(value))
		}
	}
	sort.Strings(items)
	return strings.Join(items, "&")
}

func encodeComponent(value string) string {
	var out strings.Builder
	const hexDigits = "0123456789ABCDEF"
	for _, b := range []byte(value) {
		if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') ||
			(b >= '0' && b <= '9') || strings.ContainsRune("-_.~", rune(b)) {
			out.WriteByte(b)
			continue
		}
		out.WriteByte('%')
		out.WriteByte(hexDigits[b>>4])
		out.WriteByte(hexDigits[b&15])
	}
	return out.String()
}

func hmacHex(key []byte, message string) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

func formatInt(value int64) string {
	return strconv.FormatInt(value, 10)
}
