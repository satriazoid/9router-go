package oauth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleDeviceStart_qoderLocal(t *testing.T) {
	// Qoder start is fully local (no network): PKCE + nonce + URLs.
	handler := NewOAuthHandler(nil)
	req := httptest.NewRequest("POST", "/api/oauth/device/start", strings.NewReader(`{"provider":"qoder"}`))
	rec := httptest.NewRecorder()
	handler.HandleDeviceStart(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{
		`"device_code":`, `"user_code":`, `qoder.com/device/selectAccounts`,
		`"interval":2`, `"session":`,
	} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("missing %q: %s", want, rec.Body.String())
		}
	}
}

func TestHandleDeviceStart_bad(t *testing.T) {
	handler := NewOAuthHandler(nil)
	for _, payload := range []string{`{"provider":"google"}`, `{"provider":"kiro","region":"evil;id"}`} {
		req := httptest.NewRequest("POST", "/api/oauth/device/start", strings.NewReader(payload))
		rec := httptest.NewRecorder()
		handler.HandleDeviceStart(rec, req)
		if rec.Code == http.StatusOK {
			t.Errorf("expected error for %s, got %s", payload, rec.Body.String())
		}
	}
}

func TestHandleDevicePoll_validation(t *testing.T) {
	handler := NewOAuthHandler(nil)
	for _, payload := range []string{`{}`, `{"provider":"qoder"}`, `{"provider":"nope","device_code":"x"}`} {
		req := httptest.NewRequest("POST", "/api/oauth/device/poll", strings.NewReader(payload))
		rec := httptest.NewRecorder()
		handler.HandleDevicePoll(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for %s, got %d", payload, rec.Code)
		}
	}
}

func TestKimiHeaders_deviceId(t *testing.T) {
	h := kimiHeaders("dev-1")
	if h["X-Msh-Device-Id"] != "dev-1" || h["X-Msh-Platform"] != "9router" {
		t.Errorf("bad kimi headers: %v", h)
	}
}
