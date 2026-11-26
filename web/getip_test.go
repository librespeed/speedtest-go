package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/librespeed/speedtest-go/config"
	"github.com/librespeed/speedtest-go/results"
)

func TestGetIPIgnoresForwardingHeadersFromUntrustedPeer(t *testing.T) {
	conf := &config.Config{}
	req := httptest.NewRequest(http.MethodGet, "/getIP", nil)
	req.RemoteAddr = "198.51.100.20:4321"
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	req.Header.Set("X-Real-IP", "203.0.113.10")
	req.Header.Set("CF-Connecting-IPv6", "2001:db8::1")

	rec := httptest.NewRecorder()
	getIP(conf).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got results.Result
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ProcessedString != "198.51.100.20" {
		t.Fatalf("ProcessedString = %q, want peer IP %q", got.ProcessedString, "198.51.100.20")
	}
}
