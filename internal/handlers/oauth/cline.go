package oauth

import (
	"bytes"
	"context"
	"encoding/base64"
	json "encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"9router/proxy/internal/handlerutil"
	"9router/proxy/internal/log"
)

var (
	clineOAuthAuthorizeURL = "https://api.cline.bot/api/v1/auth/authorize"
	clineOAuthTokenURL     = "https://api.cline.bot/api/v1/auth/token"
)

// clineProvider normalizes "cline" and "clinepass" to a supported provider ID.
func clineProvider(p string) (string, bool) {
	switch p {
	case "clinepass":
		return "clinepass", true
	case "", "cline":
		return "cline", true
	default:
		return "", false
	}
}

// HandleClineAuthorize returns the Cline (api.cline.bot) authorization URL with PKCE challenge.
// GET /api/oauth/cline/authorize?provider=cline|clinepass
// Mirrors upstream 9router: client_type=extension; the callback follows the
// dashboard host (override via ?redirect_uri=).
func (h *OAuthHandler) HandleClineAuthorize(w http.ResponseWriter, r *http.Request) {
	provider, ok := clineProvider(r.URL.Query().Get("provider"))
	if !ok {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "unsupported provider (want cline|clinepass)")
		return
	}

	state := r.URL.Query().Get("state")
	if state == "" {
		state = randomString(32)
	}
	verifier := randomString(64)
	challenge := sha256Base64(verifier)
	redirectURI := callbackRedirectURI(r)

	params := url.Values{
		"client_type":  {"extension"},
		"callback_url": {redirectURI},
		"redirect_uri": {redirectURI},
	}
	authURL := clineOAuthAuthorizeURL + "?" + params.Encode()

	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"url":           authURL,
		"authUrl":       authURL,
		"state":         state,
		"codeVerifier":  verifier,
		"codeChallenge": challenge,
		"redirectUri":   redirectURI,
		"flowType":      "authorization_code",
		"callbackPath":  "/callback",
		"provider":      provider,
	})
}

// HandleClineExchange exchanges the Cline authorization code for tokens and stores the connection.
// POST /api/oauth/cline/exchange
func (h *OAuthHandler) HandleClineExchange(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Provider     string `json:"provider"`
		Code         string `json:"code"`
		CodeVerifier string `json:"codeVerifier"`
		RedirectURI  string `json:"redirectUri"`
		RedirectUri  string `json:"redirect_uri"`
		State        string `json:"state"`
		Name         string `json:"name"`
	}
	if raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)); err == nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, &body)
	}

	provider, ok := clineProvider(body.Provider)
	if !ok {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "unsupported provider (want cline|clinepass)")
		return
	}
	if body.Code == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "missing code parameter")
		return
	}
	redirectURI := exchangeRedirectURI(r, body.RedirectURI, body.RedirectUri)

	tokenURL := clineOAuthTokenURL
	accessToken, refreshToken := "", ""
	if acc, ref, ok := decodeClineCode(body.Code); ok {
		accessToken, refreshToken = acc, ref
	} else {
		if body.CodeVerifier == "" {
			handlerutil.WriteJSONError(w, http.StatusBadRequest, "missing codeVerifier parameter")
			return
		}
		acc, ref, err := exchangeClineCode(r.Context(), tokenURL, body.Code, body.CodeVerifier, redirectURI)
		if err != nil {
			log.Warn("oauth", "cline token exchange non-200", "provider", provider, "error", err)
			handlerutil.WriteJSONError(w, http.StatusBadGateway, fmt.Sprintf("Cline token exchange failed: %v", err))
			return
		}
		accessToken, refreshToken = acc, ref
	}
	if accessToken == "" {
		handlerutil.WriteJSONError(w, http.StatusBadGateway, "missing accessToken in token response")
		return
	}

	connName := body.Name
	if connName == "" {
		connName = map[string]string{"cline": "Cline", "clinepass": "ClinePass"}[provider]
	}
	connID := provider + "-" + shortHash(accessToken)

	now := currentTimestamp()
	dataMap := map[string]any{
		"apiKey":      accessToken,
		"accessToken": accessToken,
	}
	if refreshToken != "" {
		dataMap["refreshToken"] = refreshToken
	}
	dataBytes, err := json.Marshal(dataMap)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to marshal connection data")
		return
	}

	if h.Repo != nil && h.Repo.RawDB() != nil {
		var existing string
		err := h.Repo.RawDB().QueryRow("SELECT data FROM providerConnections WHERE id = ?", connID).Scan(&existing)
		if err == nil && existing != "" {
			if refreshToken == "" {
				var old map[string]any
				if jerr := json.Unmarshal([]byte(existing), &old); jerr == nil {
					if rt, ok := old["refreshToken"].(string); ok && rt != "" {
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
				"INSERT INTO providerConnections (id, provider, authType, name, isActive, data, createdAt, updatedAt) VALUES (?, ?, 'oauth', ?, 1, ?, ?, ?)",
				connID, provider, connName, string(dataBytes), now, now,
			)
		}
		if err != nil {
			log.Error("oauth", "save cline connection failed", "conn", connID, "error", err)
			handlerutil.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to save connection: %v", err))
			return
		}
	}

	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"status":       "authorized",
		"id":           connID,
		"connectionId": connID,
		"provider":     provider,
		"name":         connName,
	})
}

// decodeClineCode mirrors upstream: Cline embeds the token bundle as base64
// JSON in the code param ({accessToken, refreshToken, ...}).
func decodeClineCode(code string) (access, refresh string, ok bool) {
	padded := code
	if m := len(padded) % 4; m != 0 {
		padded += strings.Repeat("=", 4-m)
	}
	raw, err := base64.StdEncoding.DecodeString(padded)
	if err != nil {
		return "", "", false
	}
	s := string(raw)
	if i := strings.LastIndex(s, "}"); i >= 0 {
		s = s[:i+1]
	} else {
		return "", "", false
	}
	var bundle struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
	}
	if err := json.Unmarshal([]byte(s), &bundle); err != nil || bundle.AccessToken == "" {
		return "", "", false
	}
	return bundle.AccessToken, bundle.RefreshToken, true
}

// exchangeClineCode POSTs the authorization code to Cline's token endpoint
// using the snake_case extension contract.
func exchangeClineCode(ctx context.Context, tokenURL, code, verifier, redirectURI string) (access, refresh string, err error) {
	reqMap := map[string]string{
		"code":          code,
		"code_verifier": verifier,
		"redirect_uri":  redirectURI,
		"grant_type":    "authorization_code",
		"client_type":   "extension",
	}
	reqBody, err := json.Marshal(reqMap)
	if err != nil {
		return "", "", err
	}
	tokenReq, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, bytes.NewReader(reqBody))
	if err != nil {
		return "", "", err
	}
	tokenReq.Header.Set("Content-Type", "application/json")
	tokenReq.Header.Set("Accept", "application/json")
	tokenReq.Header.Set("User-Agent", "Cline/3.0.61")
	tokenReq.Header.Set("X-CLIENT-TYPE", "cline-cli")
	tokenReq.Header.Set("X-CLIENT-VERSION", "3.0.61")
	tokenReq.Header.Set("X-CORE-VERSION", "3.0.61")
	tokenReq.Header.Set("X-PLATFORM", "cli")
	tokenReq.Header.Set("X-PLATFORM-VERSION", "3.0.61")

	client := &http.Client{Timeout: 15 * time.Second}
	tokenResp, err := client.Do(tokenReq)
	if err != nil {
		return "", "", err
	}
	defer tokenResp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(tokenResp.Body, 1<<20))
	if err != nil {
		return "", "", err
	}
	if tokenResp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("status %d: %s", tokenResp.StatusCode, string(respBody))
	}
	var parsed struct {
		Success bool `json:"success"`
		Error   any  `json:"error"`
		Data    struct {
			AccessToken  string `json:"accessToken"`
			RefreshToken string `json:"refreshToken"`
		} `json:"data"`
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", "", err
	}
	if !parsed.Success && parsed.Error != nil && fmt.Sprint(parsed.Error) != "" && fmt.Sprint(parsed.Error) != "<nil>" {
		return "", "", fmt.Errorf("%v", parsed.Error)
	}
	access = parsed.Data.AccessToken
	if access == "" {
		access = parsed.AccessToken
	}
	refresh = parsed.Data.RefreshToken
	if refresh == "" {
		refresh = parsed.RefreshToken
	}
	return access, refresh, nil
}
