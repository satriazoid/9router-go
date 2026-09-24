package oauth

import (
	"bytes"
	"context"
	"crypto/rand"
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

// pkceConfig describes a standard OAuth2 authorization_code+PKCE provider.
// Mirrors the upstream 9router provider modules (src/lib/oauth/providers/*).
type pkceConfig struct {
	providers      []string
	clientID       string
	authorizeURL   string
	tokenURL       string
	scope          string
	extraAuth      map[string]string
	exchangeJSON   bool
	includeState   bool
	userInfoPath   string // appended to baseURL, e.g. GitLab /api/v4/user
	discoverIssuer string // OIDC issuer for runtime discovery (xAI)
	connPrefix     string
	display        string
	emailFromIDTok bool
}

var pkceProviders = map[string]*pkceConfig{
	"claude": {
		providers:    []string{"claude"},
		clientID:     "9d1c250a-e61b-44d9-88ed-5944d1962f5e",
		authorizeURL: "https://claude.ai/oauth/authorize",
		tokenURL:     "https://api.anthropic.com/v1/oauth/token",
		scope:        "org:create_api_key user:profile user:inference",
		extraAuth:    map[string]string{"code": "true"},
		exchangeJSON: true,
		includeState: true,
		connPrefix:   "claude-",
		display:      "Claude",
	},
	"codex": {
		providers:    []string{"codex"},
		clientID:     "app_EMoamEEZ73f0CkXaXp7hrann",
		authorizeURL: "https://auth.openai.com/oauth/authorize",
		tokenURL:     "https://auth.openai.com/oauth/token",
		scope:        "openid profile email offline_access",
		extraAuth: map[string]string{
			"id_token_add_organizations": "true",
			"codex_cli_simplified_flow":  "true",
			"originator":                 "codex_cli_rs",
		},
		connPrefix:     "cx-",
		display:        "Codex",
		emailFromIDTok: true,
	},
	"xai": {
		providers:      []string{"xai"},
		clientID:       "b1a00492-073a-47ea-816f-4c329264a828",
		authorizeURL:   "https://auth.x.ai/oauth2/authorize",
		tokenURL:       "https://auth.x.ai/oauth2/token",
		scope:          "openid profile email offline_access grok-cli:access api:access",
		extraAuth:      map[string]string{"plan": "generic", "referrer": "cli-proxy-api"},
		discoverIssuer: "https://auth.x.ai",
		connPrefix:     "xai-",
		display:        "xAI",
		emailFromIDTok: true,
	},
	"gitlab": {
		providers:    []string{"gitlab"},
		authorizeURL: "https://gitlab.com/oauth/authorize",
		tokenURL:     "https://gitlab.com/oauth/token",
		scope:        "api read_user",
		userInfoPath: "/api/v4/user",
		connPrefix:   "gl-",
		display:      "GitLab",
	},
}

func pkceLookup(provider string) *pkceConfig {
	if cfg, ok := pkceProviders[provider]; ok {
		return cfg
	}
	return nil
}

// HandlePKCEAuthorize returns the authorization URL + PKCE challenge for a provider.
// GET /api/oauth/pkce/authorize?provider=claude|codex|xai|gitlab&redirect_uri=...&state=...
func (h *OAuthHandler) HandlePKCEAuthorize(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	cfg := pkceLookup(provider)
	if cfg == nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "unsupported provider (want claude|codex|xai|gitlab)")
		return
	}
	redirectURI := callbackRedirectURI(r)
	state := r.URL.Query().Get("state")
	if state == "" {
		state = randomString(32)
	}
	verifier := pkceVerifier()
	challenge := sha256Base64(verifier)

	params := url.Values{
		"response_type":         {"code"},
		"client_id":             {cfg.clientID},
		"redirect_uri":          {redirectURI},
		"scope":                 {cfg.scope},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {state},
	}
	// GitLab: allow self-hosted base + user OAuth app credentials.
	authorizeURL, tokenURL := cfg.authorizeURL, cfg.tokenURL
	if provider == "gitlab" {
		base := strings.TrimSuffix(r.URL.Query().Get("baseUrl"), "/")
		if base == "" {
			base = "https://gitlab.com"
		}
		authorizeURL, tokenURL = base+"/oauth/authorize", base+"/oauth/token"
		if cid := r.URL.Query().Get("clientId"); cid != "" {
			params.Set("client_id", cid)
		}
	}
	for k, v := range cfg.extraAuth {
		params.Set(k, v)
	}
	// xAI: fresh nonce per request + runtime OIDC discovery.
	if provider == "xai" {
		nonce := make([]byte, 16)
		if _, err := rand.Read(nonce); err == nil {
			params.Set("nonce", fmt.Sprintf("%x", nonce))
		}
		if disc, err := discoverOIDCEndpoints(cfg.discoverIssuer); err == nil {
			authorizeURL, tokenURL = disc.authorizeURL, disc.tokenURL
		}
	}

	authURL := authorizeURL + "?" + params.Encode()
	_ = tokenURL
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"url":           authURL,
		"authUrl":       authURL,
		"state":         state,
		"codeVerifier":  verifier,
		"codeChallenge": challenge,
		"redirectUri":   redirectURI,
		"flowType":      "authorization_code_pkce",
		"provider":      provider,
	})
}

// HandlePKCEExchange exchanges the authorization code and stores the connection.
// POST /api/oauth/pkce/exchange
func (h *OAuthHandler) HandlePKCEExchange(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Provider     string `json:"provider"`
		Code         string `json:"code"`
		CodeVerifier string `json:"codeVerifier"`
		RedirectURI  string `json:"redirectUri"`
		RedirectUri  string `json:"redirect_uri"`
		State        string `json:"state"`
		Name         string `json:"name"`
		BaseURL      string `json:"baseUrl"`
		ClientID     string `json:"clientId"`
		ClientSecret string `json:"clientSecret"`
	}
	if raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)); err == nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, &body)
	}
	cfg := pkceLookup(body.Provider)
	if cfg == nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "unsupported provider (want claude|codex|xai|gitlab)")
		return
	}
	code := body.Code
	if i := strings.Index(code, "#"); i >= 0 { // Claude appends state after #
		code = code[:i]
	}
	if code == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "missing code parameter")
		return
	}
	if body.CodeVerifier == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "missing codeVerifier parameter")
		return
	}
	redirectURI := exchangeRedirectURI(r, body.RedirectURI, body.RedirectUri)
	tokenURL := cfg.tokenURL
	clientID := cfg.clientID
	base := ""
	if body.Provider == "gitlab" {
		base = strings.TrimSuffix(body.BaseURL, "/")
		if base == "" {
			base = "https://gitlab.com"
		}
		tokenURL = base + "/oauth/token"
		if body.ClientID != "" {
			clientID = body.ClientID
		}
	}
	if body.Provider == "xai" {
		if disc, err := discoverOIDCEndpoints(cfg.discoverIssuer); err == nil {
			tokenURL = disc.tokenURL
		}
	}

	var tokenReq *http.Request
	if cfg.exchangeJSON { // Claude: JSON body incl. state
		payload := map[string]string{
			"code":          code,
			"grant_type":    "authorization_code",
			"client_id":     clientID,
			"redirect_uri":  redirectURI,
			"code_verifier": body.CodeVerifier,
		}
		if cfg.includeState {
			payload["state"] = body.State
		}
		raw, _ := json.Marshal(payload)
		var err error
		tokenReq, err = http.NewRequestWithContext(r.Context(), http.MethodPost, tokenURL, bytes.NewReader(raw))
		if err != nil {
			handlerutil.WriteJSONError(w, http.StatusInternalServerError, "create token request failed")
			return
		}
		tokenReq.Header.Set("Content-Type", "application/json")
	} else { // codex / xai / gitlab: form body
		form := url.Values{
			"grant_type":    {"authorization_code"},
			"client_id":     {clientID},
			"code":          {code},
			"redirect_uri":  {redirectURI},
			"code_verifier": {body.CodeVerifier},
		}
		if body.ClientSecret != "" {
			form.Set("client_secret", body.ClientSecret)
		}
		var err error
		tokenReq, err = http.NewRequestWithContext(r.Context(), http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
		if err != nil {
			handlerutil.WriteJSONError(w, http.StatusInternalServerError, "create token request failed")
			return
		}
		tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	tokenReq.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	tokenResp, err := client.Do(tokenReq)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadGateway, fmt.Sprintf("token exchange failed: %v", err))
		return
	}
	defer tokenResp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(tokenResp.Body, 1<<20))
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadGateway, "failed to read token response")
		return
	}
	if tokenResp.StatusCode != http.StatusOK {
		log.Warn("oauth", "pkce token exchange non-200", "provider", body.Provider, "status", tokenResp.StatusCode, "body", string(respBody))
		handlerutil.WriteJSONError(w, http.StatusBadGateway, fmt.Sprintf("token exchange returned status %d: %s", tokenResp.StatusCode, string(respBody)))
		return
	}
	var tokens struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		Scope        string `json:"scope"`
		IDToken      string `json:"id_token"`
	}
	if err := json.Unmarshal(respBody, &tokens); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadGateway, "failed to decode token response")
		return
	}
	if tokens.AccessToken == "" {
		handlerutil.WriteJSONError(w, http.StatusBadGateway, "missing access_token in token response")
		return
	}

	email, extra := "", map[string]any{}
	if cfg.emailFromIDTok {
		email = extractEmailFromJWT(tokens.IDToken)
		if tokens.IDToken != "" {
			extra["idToken"] = tokens.IDToken
		}
	}
	if cfg.userInfoPath != "" && base != "" { // GitLab: fetch user profile
		if u, err := fetchOAuthUserInfo(r.Context(), base+cfg.userInfoPath, tokens.AccessToken); err == nil {
			if v, _ := u["email"].(string); v != "" {
				email = v
			} else if v, _ := u["public_email"].(string); v != "" {
				email = v
			}
			extra["gitlabUser"] = u
			extra["gitlabBaseUrl"] = base
		}
	}

	connName := connectionDisplayName(body.Provider, body.Name, email, cfg.display)
	connID := cfg.connPrefix + shortHash(tokens.AccessToken)
	now := currentTimestamp()
	dataMap := map[string]any{"apiKey": tokens.AccessToken, "accessToken": tokens.AccessToken}
	if tokens.RefreshToken != "" {
		dataMap["refreshToken"] = tokens.RefreshToken
	}
	if email != "" {
		dataMap["email"] = email
	}
	for k, v := range extra {
		dataMap[k] = v
	}
	if tokens.ExpiresIn > 0 {
		dataMap["expiresAt"] = time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second).UTC().Format(time.RFC3339)
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
			if tokens.RefreshToken == "" {
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
				connID, body.Provider, connName, string(dataBytes), now, now,
			)
		}
		if err != nil {
			handlerutil.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to save connection: %v", err))
			return
		}
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"status": "authorized", "id": connID, "connectionId": connID,
		"provider": body.Provider, "name": connName, "email": email,
	})
}

// pkceVerifier generates a PKCE code verifier (unreserved chars only).
func pkceVerifier() string {
	b := make([]byte, 48)
	if _, err := rand.Read(b); err != nil {
		return randomString(64)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

type oidcEndpoints struct{ authorizeURL, tokenURL string }

// discoverOIDCEndpoints fetches OIDC discovery, falling back to static config.
func discoverOIDCEndpoints(issuer string) (oidcEndpoints, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(strings.TrimSuffix(issuer, "/") + "/.well-known/openid-configuration")
	if err != nil {
		return oidcEndpoints{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil || resp.StatusCode != http.StatusOK {
		return oidcEndpoints{}, fmt.Errorf("discovery status %d", resp.StatusCode)
	}
	var doc struct {
		AuthorizationEndpoint string `json:"authorization_endpoint"`
		TokenEndpoint         string `json:"token_endpoint"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil || doc.AuthorizationEndpoint == "" || doc.TokenEndpoint == "" {
		return oidcEndpoints{}, fmt.Errorf("bad discovery doc")
	}
	if !strings.HasPrefix(doc.AuthorizationEndpoint, "https://") || !strings.HasPrefix(doc.TokenEndpoint, "https://") {
		return oidcEndpoints{}, fmt.Errorf("non-https discovery endpoints")
	}
	return oidcEndpoints{doc.AuthorizationEndpoint, doc.TokenEndpoint}, nil
}

// fetchOAuthUserInfo GETs a bearer-authenticated userinfo endpoint.
func fetchOAuthUserInfo(ctx context.Context, userInfoURL, accessToken string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userInfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo status %d", resp.StatusCode)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}
