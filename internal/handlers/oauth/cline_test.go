package oauth

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleClineAuthorize(t *testing.T) {
	handler := NewOAuthHandler(nil)
	req := httptest.NewRequest("GET", "/api/oauth/cline/authorize?provider=clinepass", nil)
	rec := httptest.NewRecorder()
	handler.HandleClineAuthorize(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`"authUrl":"https://api.cline.bot/api/v1/auth/authorize?`,
		`client_type=extension`,
		// Callback follows the request host (httptest => example.com), not a hardcoded port.
		`example.com%2Fcallback`,
		`"state":`, `"codeVerifier":`, `"codeChallenge":`,
		`"flowType":"authorization_code"`, `"provider":"clinepass"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("authorize response missing %q: %s", want, body)
		}
	}
}

func TestHandleClineAuthorize_redirectURIOverride(t *testing.T) {
	handler := NewOAuthHandler(nil)
	req := httptest.NewRequest("GET", "/api/oauth/cline/authorize?provider=cline&redirect_uri=http://localhost:9999/cb", nil)
	rec := httptest.NewRecorder()
	handler.HandleClineAuthorize(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, `localhost%3A9999%2Fcb`) {
		t.Errorf("explicit redirect_uri override not honored: %s", body)
	}
}

func TestHandleClineAuthorize_redirectURISchemeRejected(t *testing.T) {
	handler := NewOAuthHandler(nil)
	req := httptest.NewRequest("GET", "/api/oauth/cline/authorize?provider=cline&redirect_uri=javascript:alert(1)", nil)
	rec := httptest.NewRecorder()
	handler.HandleClineAuthorize(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); strings.Contains(body, `javascript`) {
		t.Errorf("non-http redirect_uri must be ignored: %s", body)
	}
}

func TestHandleClineAuthorize_badProvider(t *testing.T) {
	handler := NewOAuthHandler(nil)
	req := httptest.NewRequest("GET", "/api/oauth/cline/authorize?provider=google", nil)
	rec := httptest.NewRecorder()
	handler.HandleClineAuthorize(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandleClineExchange_missing(t *testing.T) {
	handler := NewOAuthHandler(nil)
	req := httptest.NewRequest("POST", "/api/oauth/cline/exchange", strings.NewReader(`{"provider":"cline"}`))
	rec := httptest.NewRecorder()
	handler.HandleClineExchange(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}
func TestHandleClineExchange_success(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/token" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success":true,"data":{"accessToken":"acc-123","refreshToken":"ref-456","expiresIn":3600}}`))
	}))
	defer mock.Close()

	old := clineOAuthTokenURL
	clineOAuthTokenURL = mock.URL + "/api/v1/auth/token"
	defer func() { clineOAuthTokenURL = old }()

	handler := NewOAuthHandler(nil)
	req := httptest.NewRequest("POST", "/api/oauth/cline/exchange",
		strings.NewReader(`{"provider":"clinepass","code":"c","codeVerifier":"v","redirectUri":"http://localhost:8080/callback"}`))
	rec := httptest.NewRecorder()
	handler.HandleClineExchange(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{`"status":"authorized"`, `"provider":"clinepass"`, `"connectionId":`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("exchange response missing %q: %s", want, rec.Body.String())
		}
	}
}

func TestHandleClineExchange_base64Code(t *testing.T) {
	// Upstream embeds the token bundle as base64 JSON in the code param.
	bundle := base64.StdEncoding.EncodeToString([]byte(`{"accessToken":"b64-acc","refreshToken":"b64-ref","email":"u@x.io"}`))
	handler := NewOAuthHandler(nil)
	req := httptest.NewRequest("POST", "/api/oauth/cline/exchange",
		strings.NewReader(`{"provider":"cline","code":"`+bundle+`"}`))
	rec := httptest.NewRecorder()
	handler.HandleClineExchange(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"status":"authorized"`) {
		t.Errorf("expected authorized, got %s", rec.Body.String())
	}
}
