package oauth

import (
	json "encoding/json/v2"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"9router/proxy/internal/handlerutil"
	"9router/proxy/internal/log"
	"9router/proxy/internal/providers"
)

var (
	googleOAuthAuthURL  = "https://accounts.google.com/o/oauth2/v2/auth"
	googleOAuthTokenURL = "https://oauth2.googleapis.com/token"
)

var defaultAntigravityScopes = []string{
	"https://www.googleapis.com/auth/cloud-platform",
	"https://www.googleapis.com/auth/userinfo.email",
	"https://www.googleapis.com/auth/userinfo.profile",
	"openid",
}

func getAntigravityOAuthConfig() (clientID, clientSecret, tokenURL string) {
	clientID = "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com"
	clientSecret = "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"
	tokenURL = googleOAuthTokenURL

	if cfg, ok := providers.KnownOAuthConfigs["antigravity"]; ok {
		if cfg.ClientID != "" {
			clientID = cfg.ClientID
		}
		if cfg.ClientSecret != "" {
			clientSecret = cfg.ClientSecret
		}
		if cfg.TokenURL != "" && googleOAuthTokenURL == "https://oauth2.googleapis.com/token" {
			tokenURL = cfg.TokenURL
		}
	}
	if v := os.Getenv("ANTIGRAVITY_OAUTH_CLIENT_ID"); v != "" {
		clientID = v
	}
	if v := os.Getenv("ANTIGRAVITY_OAUTH_CLIENT_SECRET"); v != "" {
		clientSecret = v
	}
	return
}

func getAntigravityRedirectURI(r *http.Request) string {
	if redirectURI := r.URL.Query().Get("redirect_uri"); redirectURI != "" {
		return redirectURI
	}
	if envURI := os.Getenv("ANTIGRAVITY_REDIRECT_URI"); envURI != "" {
		return envURI
	}
	if r.Host != "" {
		scheme := "http"
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		return scheme + "://" + r.Host + "/api/oauth/antigravity/callback"
	}
	return "http://localhost:8080/api/oauth/antigravity/callback"
}

// HandleAntigravityAuthorize returns the Google OAuth authorization URL for Antigravity.
// GET /api/oauth/antigravity/authorize
func (h *OAuthHandler) HandleAntigravityAuthorize(w http.ResponseWriter, r *http.Request) {
	clientID, _, _ := getAntigravityOAuthConfig()
	redirectURI := getAntigravityRedirectURI(r)

	scopes := defaultAntigravityScopes
	if customScope := r.URL.Query().Get("scope"); customScope != "" {
		scopes = strings.Split(customScope, " ")
	}

	state := r.URL.Query().Get("state")
	if state == "" {
		state = randomString(32)
	}

	params := url.Values{
		"client_id":     {clientID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"scope":         {strings.Join(scopes, " ")},
		"access_type":   {"offline"},
		"prompt":        {"consent"},
		"state":         {state},
	}

	authURL := googleOAuthAuthURL + "?" + params.Encode()

	if r.URL.Query().Get("redirect") == "true" {
		http.Redirect(w, r, authURL, http.StatusFound)
		return
	}

	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"url":         authURL,
		"redirectUrl": authURL,
		"state":       state,
	})
}

// HandleAntigravityCallback exchanges the Google authorization code for tokens and stores the provider connection in DB.
// GET /api/oauth/antigravity/callback
// POST /api/oauth/antigravity/callback
func (h *OAuthHandler) HandleAntigravityCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	redirectURI := r.URL.Query().Get("redirect_uri")

	if code == "" && r.Method == http.MethodPost {
		var body struct {
			Code             string `json:"code"`
			RedirectURI      string `json:"redirect_uri"`
			RedirectUriCamel string `json:"redirectUri"`
			State            string `json:"state"`
		}
		if raw, err := io.ReadAll(r.Body); err == nil && len(raw) > 0 {
			_ = json.Unmarshal(raw, &body)
			code = body.Code
			if state == "" && body.State != "" {
				state = body.State
			}
			if redirectURI == "" {
				if body.RedirectURI != "" {
					redirectURI = body.RedirectURI
				} else {
					redirectURI = body.RedirectUriCamel
				}
			}
		}
	}

	if errParam := r.URL.Query().Get("error"); errParam != "" {
		desc := r.URL.Query().Get("error_description")
		h.respondAntigravityError(w, r, http.StatusBadRequest, fmt.Sprintf("oauth error: %s (%s)", errParam, desc), state)
		return
	}

	code = cleanAuthCode(code)
	if code == "" {
		h.respondAntigravityError(w, r, http.StatusBadRequest, "missing code parameter", state)
		return
	}

	if redirectURI == "" {
		redirectURI = getAntigravityRedirectURI(r)
	}
	redirectURI = strings.TrimSpace(redirectURI)
	clientID, clientSecret, tokenURL := getAntigravityOAuthConfig()

	form := url.Values{
		"code":          {code},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"redirect_uri":  {redirectURI},
		"grant_type":    {"authorization_code"},
	}

	tokenReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		h.respondAntigravityError(w, r, http.StatusInternalServerError, fmt.Sprintf("create token request failed: %v", err), state)
		return
	}
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// No custom UA: the handler runs server-side against Google token
	// endpoints, and any router brand here is pure self-identification.
	// Go's default UA ("Go-http-client/2.0") is unbranded and sufficient.

	client := &http.Client{Timeout: 15 * time.Second}
	tokenResp, err := client.Do(tokenReq)
	if err != nil {
		log.Error("oauth", "antigravity token exchange failed", "error", err)
		h.respondAntigravityError(w, r, http.StatusBadGateway, fmt.Sprintf("token exchange failed: %v", err), state)
		return
	}
	defer tokenResp.Body.Close()

	respBody, err := io.ReadAll(tokenResp.Body)
	if err != nil {
		h.respondAntigravityError(w, r, http.StatusBadGateway, "failed to read token response", state)
		return
	}

	if tokenResp.StatusCode != http.StatusOK {
		log.Warn("oauth", "antigravity token exchange non-200", "status", tokenResp.StatusCode, "body", string(respBody))
		h.respondAntigravityError(w, r, http.StatusBadGateway, fmt.Sprintf("token exchange returned status %d: %s", tokenResp.StatusCode, string(respBody)), state)
		return
	}

	var tokenData struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
		Scope        string `json:"scope"`
		IDToken      string `json:"id_token"`
	}

	if err := json.Unmarshal(respBody, &tokenData); err != nil {
		log.Error("oauth", "antigravity decode token response failed", "error", err)
		h.respondAntigravityError(w, r, http.StatusBadGateway, "failed to decode token response", state)
		return
	}

	if tokenData.AccessToken == "" {
		h.respondAntigravityError(w, r, http.StatusBadGateway, "missing access_token in token response", state)
		return
	}

	email := ""
	if tokenData.IDToken != "" {
		email = extractEmailFromJWT(tokenData.IDToken)
	}

	connID := ""
	if email != "" {
		connID = "ag-" + shortHash(email)
	} else if tokenData.RefreshToken != "" {
		connID = "ag-" + shortHash(tokenData.RefreshToken)
	} else {
		connID = "ag-" + randomString(12)
	}

	connName := connectionDisplayName("antigravity", "", email, "Antigravity")

	now := currentTimestamp()
	dataMap := map[string]any{
		"apiKey":      tokenData.AccessToken,
		"accessToken": tokenData.AccessToken,
		"tokenType":   tokenData.TokenType,
	}
	if tokenData.RefreshToken != "" {
		dataMap["refreshToken"] = tokenData.RefreshToken
	}
	if email != "" {
		dataMap["email"] = email
	}
	if tokenData.IDToken != "" {
		dataMap["idToken"] = tokenData.IDToken
	}
	if tokenData.Scope != "" {
		dataMap["scope"] = tokenData.Scope
	}
	if tokenData.ExpiresIn > 0 {
		dataMap["expiresAt"] = time.Now().Add(time.Duration(tokenData.ExpiresIn) * time.Second).UTC().Format(time.RFC3339)
	}

	dataBytes, err := json.Marshal(dataMap)
	if err != nil {
		h.respondAntigravityError(w, r, http.StatusInternalServerError, "failed to marshal connection data", state)
		return
	}

	if h.Repo != nil && h.Repo.RawDB() != nil {
		var existingDataStr string
		err := h.Repo.RawDB().QueryRow("SELECT data FROM providerConnections WHERE id = ?", connID).Scan(&existingDataStr)
		if err == nil && existingDataStr != "" {
			if tokenData.RefreshToken == "" {
				var existingData map[string]any
				if err := json.Unmarshal([]byte(existingDataStr), &existingData); err == nil {
					if rt, ok := existingData["refreshToken"].(string); ok && rt != "" {
						dataMap["refreshToken"] = rt
					}
				}
				dataBytes, _ = json.Marshal(dataMap)
			}
			_, err = h.Repo.RawDB().Exec(
				"UPDATE providerConnections SET name = ?, data = ?, updatedAt = ? WHERE id = ?",
				connName, string(dataBytes), now, connID,
			)
		} else {
			_, err = h.Repo.RawDB().Exec(
				"INSERT INTO providerConnections (id, provider, authType, name, isActive, data, createdAt, updatedAt) VALUES (?, 'antigravity', 'oauth', ?, 1, ?, ?, ?)",
				connID, connName, string(dataBytes), now, now,
			)
		}
		if err != nil {
			log.Error("oauth", "save antigravity connection failed", "conn", connID, "error", err)
			h.respondAntigravityError(w, r, http.StatusInternalServerError, fmt.Sprintf("failed to save connection: %v", err), state)
			return
		}
	}

	h.respondAntigravitySuccess(w, r, connID, connName, email, state)
}

func cleanAuthCode(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}
	// If the user pasted a full URL or query string like "https://...?code=..." or "code=..."
	if strings.Contains(code, "code=") {
		if u, err := url.Parse(code); err == nil && u.Query().Get("code") != "" {
			code = u.Query().Get("code")
		} else {
			parts := strings.Split(code, "code=")
			if len(parts) > 1 {
				sub := strings.Split(parts[1], "&")
				code = sub[0]
			}
		}
	}
	// Decode percent encoding until fully unescaped so that "4%2F..." or "4%252F..." becomes "4/..."
	for strings.Contains(code, "%") {
		unescaped, err := url.QueryUnescape(code)
		if err != nil || unescaped == code {
			break
		}
		code = unescaped
	}
	return strings.TrimSpace(code)
}

func wantsHTML(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}

func (h *OAuthHandler) respondAntigravityError(w http.ResponseWriter, r *http.Request, statusCode int, msg, state string) {
	if wantsHTML(r) {
		renderAntigravityCallbackError(w, statusCode, msg, state)
		return
	}
	handlerutil.WriteJSONError(w, statusCode, msg)
}

func (h *OAuthHandler) respondAntigravitySuccess(w http.ResponseWriter, r *http.Request, connID, connName, email, state string) {
	if wantsHTML(r) {
		renderAntigravityCallbackSuccess(w, connID, connName, email, state)
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"status":       "authorized",
		"id":           connID,
		"connectionId": connID,
		"provider":     "antigravity",
		"name":         connName,
		"email":        email,
	})
}

func renderAntigravityCallbackSuccess(w http.ResponseWriter, connID, connName, email, state string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	htmlBody := fmt.Sprintf(`<!DOCTYPE html>
<html lang="id">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>9router — Antigravity Connected</title>
<style>
:root{color-scheme:dark}
body{background:#0b0e14;color:#e6e9f0;font-family:ui-sans-serif,system-ui,sans-serif;margin:0;padding:32px 16px;display:flex;align-items:center;justify-content:center;min-height:80vh}
.card{max-width:480px;width:100%%;background:#131722;border:1px solid #2a3040;border-radius:12px;padding:24px;text-align:center;box-shadow:0 8px 30px rgba(0,0,0,0.5)}
h1{font-size:20px;margin:0 0 12px;color:#4ade80}
p{font-size:14px;color:#9aa3b5;margin:8px 0}
.email{font-weight:600;color:#e6e9f0}
#status{font-size:13px;color:#38bdf8;margin-top:16px}
.btn{display:inline-block;margin-top:16px;padding:10px 20px;border-radius:8px;background:#2563eb;color:#fff;text-decoration:none;font-weight:600;font-size:14px}
.btn:hover{background:#1d4ed8}
</style>
</head>
<body>
<div class="card">
<h1>✅ Antigravity Terhubung!</h1>
<p>Koneksi <span class="email">%s</span> berhasil disimpan.</p>
<p id="status">Mengirim ke dashboard — tab ini akan tertutup otomatis…</p>
<a href="/dashboard/providers/antigravity" class="btn">Kembali ke Dashboard</a>
</div>
<script>
(function(){
  var payload = { provider: "antigravity", connectionId: %q, name: %q, email: %q, success: true, at: Date.now() };
  try {
    localStorage.setItem("9router.oauth.callback.v1", JSON.stringify({ state: %q, raw: "success", at: Date.now() }));
  } catch(e) {}
  try {
    var bc = new BroadcastChannel("9router-oauth");
    bc.postMessage(payload);
    bc.close();
  } catch(e) {}
  if (window.opener) {
    try {
      window.opener.postMessage({ type: "9router-oauth-success", provider: "antigravity", email: %q }, "*");
    } catch(e) {}
  }
  var n = 3;
  var statusEl = document.getElementById("status");
  var timer = setInterval(function(){
    n -= 1;
    if (n <= 0) {
      clearInterval(timer);
      try { window.close(); } catch(e) {}
      if (statusEl) statusEl.textContent = "Koneksi berhasil diproses. Tab ini boleh ditutup.";
    } else {
      if (statusEl) statusEl.textContent = "Menutup tab ini dalam " + n + "…";
    }
  }, 1000);
})();
</script>
</body>
</html>`, html.EscapeString(connName), connID, connName, email, state, email)
	_, _ = w.Write([]byte(htmlBody))
}

func renderAntigravityCallbackError(w http.ResponseWriter, statusCode int, errMsg, state string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(statusCode)
	htmlBody := fmt.Sprintf(`<!DOCTYPE html>
<html lang="id">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>9router — Antigravity OAuth Error</title>
<style>
:root{color-scheme:dark}
body{background:#0b0e14;color:#e6e9f0;font-family:ui-sans-serif,system-ui,sans-serif;margin:0;padding:32px 16px;display:flex;align-items:center;justify-content:center;min-height:80vh}
.card{max-width:480px;width:100%%;background:#131722;border:1px solid #2a3040;border-radius:12px;padding:24px;text-align:center;box-shadow:0 8px 30px rgba(0,0,0,0.5)}
h1{font-size:20px;margin:0 0 12px;color:#f87171}
p{font-size:14px;color:#9aa3b5;margin:8px 0}
.err{color:#fca5a5;background:rgba(239,68,68,0.1);padding:10px;border-radius:8px;font-family:monospace;font-size:12px;margin:12px 0;word-break:break-all}
.btn{display:inline-block;margin-top:16px;padding:10px 20px;border-radius:8px;background:#2563eb;color:#fff;text-decoration:none;font-weight:600;font-size:14px}
.btn:hover{background:#1d4ed8}
</style>
</head>
<body>
<div class="card">
<h1>❌ Otorisasi Antigravity Gagal</h1>
<div class="err">%s</div>
<p>Silakan coba kembali dari dashboard.</p>
<a href="/dashboard/providers/antigravity" class="btn">Kembali ke Dashboard</a>
</div>
<script>
(function(){
  try {
    localStorage.setItem("9router.oauth.callback.v1", JSON.stringify({ state: %q, error: %q, at: Date.now() }));
  } catch(e) {}
  try {
    var bc = new BroadcastChannel("9router-oauth");
    bc.postMessage({ provider: "antigravity", error: %q, at: Date.now() });
    bc.close();
  } catch(e) {}
  if (window.opener) {
    try {
      window.opener.postMessage({ type: "9router-oauth-error", provider: "antigravity", error: %q }, "*");
    } catch(e) {}
  }
})();
</script>
</body>
</html>`, html.EscapeString(errMsg), state, errMsg, errMsg, errMsg)
	_, _ = w.Write([]byte(htmlBody))
}
