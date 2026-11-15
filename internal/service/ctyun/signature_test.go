package ctyun

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestGenerateAuthorizationDeterministicAndCanonicalQuery(t *testing.T) {
	now := time.Unix(1700000000, 0)
	body := []byte(`{"model":"telechat-115b"}`)
	first, err := GenerateAuthorization(
		"https://xirang-openapi.ctyun.cn/aipaas/lm/v1/telechat/completions115?b=2&a=1",
		"app-id", "app-key", "QG", "teleai-cloud-auth-v1", body, now, 1800,
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := GenerateAuthorization(
		"https://xirang-openapi.ctyun.cn/aipaas/lm/v1/telechat/completions115?a=1&b=2",
		"app-id", "app-key", "QG", "teleai-cloud-auth-v1", body, now, 1800,
	)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("query order should not change signature: %s %s", first, second)
	}
	parts := strings.Split(first, "/")
	if len(parts) != 7 || parts[0] != "teleai-cloud-auth-v1" || parts[5] != "x-app-id" || len(parts[6]) != 64 {
		t.Fatalf("unexpected authorization value: %s", first)
	}
}

func TestCanonicalQueryEscapesAndSorts(t *testing.T) {
	values, err := url.ParseQuery("z=last+value&a=first")
	if err != nil {
		t.Fatal(err)
	}
	got := canonicalQueryString(values)
	if got != "a=first&z=last%20value" {
		t.Fatalf("unexpected canonical query: %s", got)
	}
}
