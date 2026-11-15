package util

import (
	"strings"
	"testing"
)

func TestNewIDReturnsUUIDShape(t *testing.T) {
	id, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	if len(id) != 36 || id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' {
		t.Fatalf("unexpected ID format: %q", id)
	}
	if strings.Trim(id, "0123456789abcdef-") != "" {
		t.Fatalf("unexpected characters in ID: %q", id)
	}
}
