package oauth

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	json "encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"9router/proxy/internal/handlerutil"
	"9router/proxy/internal/log"
)

var freebuffAuthBaseURL = "https://freebuff.com"
var freebuffPendingSessions sync.Map


// shortHash returns a 12-character hex SHA-256 hash of the input.
func shortHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])[:12]
}

// HandleFreebuffInitiate initiates the Freebuff device authorization flow by
// generating a secure auth code, fingerprint hash, and login URL.
// POST /api/oauth/freebuff/initiate
func (h *OAuthHandler) HandleFreebuffInitiate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		handlerutil.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Generate 16 cryptographically random bytes for the auth code
	rawBytes := make([]byte, 16)
	if _, err := rand.Read(rawBytes); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to generate random auth code")
		return
	}
	authCode := base64.RawURLEncoding.EncodeToString(rawBytes)

	// Compute SHA-256 fingerprint hash of the auth code
	hash := sha256.Sum256([]byte(authCode))
	fingerprintHash := hex.EncodeToString(hash[:])

	// Generate random UUID v4 for the fingerprint ID
	fingerprintID := uuid.New().String()

	// Default expiration to 60 minutes in the future
	expiresAt := time.Now().UTC().Add(60 * time.Minute)
	expiresAtMs := expiresAt.UnixMilli()

	loginURL := fmt.Sprintf("%s/login?auth_code=%s", freebuffAuthBaseURL, authCode)

	// Register the code with upstream Freebuff if available
	upstreamPayload, err := json.Marshal(map[string]string{
		"fingerprintId":   fingerprintID,
		"fingerprintHash": fingerprintHash,
	})
	if err == nil {
		reqURL := freebuffAuthBaseURL + "/api/auth/cli/code"
		upReq, reqErr := http.NewRequestWithContext(r.Context(), http.MethodPost, reqURL, bytes.NewReader(upstreamPayload))
		if reqErr == nil {
			upReq.Header.Set("Content-Type", "application/json")
			upReq.Header.Set("User-Agent", "codebuff-cli/0.0.138")
			client := &http.Client{Timeout: 10 * time.Second}
			resp, doErr := client.Do(upReq)
			if doErr == nil {
				defer resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					respBytes, _ := io.ReadAll(resp.Body)
					var data struct {
						FingerprintID   string `json:"fingerprintId"`
						FingerprintHash string `json:"fingerprintHash"`
						LoginURL        string `json:"loginUrl"`
						ExpiresAt       int64  `json:"expiresAt"`
					}
					if err := json.Unmarshal(respBytes, &data); err == nil {
						if data.FingerprintID != "" {
							fingerprintID = data.FingerprintID
						}
						if data.FingerprintHash != "" {
							fingerprintHash = data.FingerprintHash
						}
						if data.LoginURL != "" {
							loginURL = data.LoginURL
							if strings.Contains(loginURL, "auth_code=") {
								parts := strings.Split(loginURL, "auth_code=")
								if len(parts) > 1 {
									authCode = parts[1]
								}
							}
						}
						if data.ExpiresAt > 0 {
							expiresAtMs = data.ExpiresAt
						}
					}
				}
			}
		}
	}

	freebuffPendingSessions.Store(fingerprintID, expiresAtMs)

	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"loginUrl":        loginURL,
		"authCode":        authCode,
		"fingerprintId":   fingerprintID,
		"fingerprintHash": fingerprintHash,
		"expiresAt":       expiresAtMs,
	})
}

// HandleFreebuffPoll polls Freebuff authorization status and creates/updates provider connection in DB.
// Calls https://freebuff.com/api/auth/cli/status with method POST.
// POST /api/oauth/freebuff/poll
func (h *OAuthHandler) HandleFreebuffPoll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		handlerutil.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	defer r.Body.Close()

	var req struct {
		FingerprintID        string `json:"fingerprintId"`
		FingerprintIDSnake   string `json:"fingerprint_id"`
		FingerprintHash      string `json:"fingerprintHash"`
		FingerprintHashSnake string `json:"fingerprint_hash"`
		ExpiresAt            any    `json:"expiresAt"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	fpID := req.FingerprintID
	if fpID == "" {
		fpID = req.FingerprintIDSnake
	}
	fpHash := req.FingerprintHash
	if fpHash == "" {
		fpHash = req.FingerprintHashSnake
	}

	if fpID == "" || fpHash == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "missing fingerprintId or fingerprintHash")
		return
	}

	var expiresAtMs int64
	if v, ok := freebuffPendingSessions.Load(fpID); ok {
		if val, ok := v.(int64); ok {
			expiresAtMs = val
		}
	}
	if expiresAtMs == 0 && req.ExpiresAt != nil {
		switch v := req.ExpiresAt.(type) {
		case float64:
			expiresAtMs = int64(v)
		case int64:
			expiresAtMs = v
		case string:
			expiresAtMs, _ = strconv.ParseInt(v, 10, 64)
		}
	}
	if expiresAtMs == 0 {
		expiresAtMs = time.Now().UTC().Add(60 * time.Minute).UnixMilli()
	}

	params := url.Values{}
	params.Set("fingerprintId", fpID)
	params.Set("fingerprintHash", fpHash)
	params.Set("expiresAt", strconv.FormatInt(expiresAtMs, 10))

	targetURL := fmt.Sprintf("%s/api/auth/cli/status?%s", freebuffAuthBaseURL, params.Encode())

	upReq, err := http.NewRequestWithContext(r.Context(), http.MethodGet, targetURL, nil)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("create status request failed: %v", err))
		return
	}
	upReq.Header.Set("User-Agent", "codebuff-cli/0.0.138")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(upReq)
	if err != nil {
		log.Error("oauth", "freebuff poll status request failed", "error", err)
		handlerutil.WriteJSONError(w, http.StatusBadGateway, fmt.Sprintf("freebuff status failed: %v", err))
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadGateway, "failed to read freebuff status response")
		return
	}

	trimmedBody := bytes.TrimSpace(respBody)

	// Upstream returns 401 {"error":"Authentication failed"}, 204 No Content, or empty body while pending approval
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusNoContent || len(trimmedBody) == 0 {
		handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
			"status": "pending",
		})
		return
	}

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		freebuffPendingSessions.Delete(fpID)
		handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
			"status": "expired",
		})
		return
	}

	var rawUpstream map[string]any
	_ = json.Unmarshal(respBody, &rawUpstream)

	var upstream struct {
		Status         string `json:"status"`
		AuthToken      string `json:"authToken"`
		AuthTokenSnake string `json:"auth_token"`
		AccessToken    string `json:"accessToken"`
		Token          string `json:"token"`
		Email          string `json:"email"`
		Name           string `json:"name"`
		Message        string `json:"message"`
		Error          string `json:"error"`
	}
	if err := json.Unmarshal(respBody, &upstream); err != nil {
		log.Error("oauth", "freebuff poll unmarshal failed", "error", err, "status", resp.StatusCode, "body", string(respBody))
		if resp.StatusCode != http.StatusOK {
			handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
				"status": "pending",
			})
			return
		}
		handlerutil.WriteJSONError(w, http.StatusBadGateway, "failed to parse freebuff status response")
		return
	}

	authToken := upstream.AuthToken
	if authToken == "" {
		authToken = upstream.AuthTokenSnake
	}
	if authToken == "" {
		authToken = upstream.AccessToken
	}
	if authToken == "" {
		authToken = upstream.Token
	}
	if authToken == "" && rawUpstream != nil {
		if at, ok := rawUpstream["accessToken"].(string); ok && at != "" {
			authToken = at
		} else if at, ok := rawUpstream["access_token"].(string); ok && at != "" {
			authToken = at
		} else if at, ok := rawUpstream["token"].(string); ok && at != "" {
			authToken = at
		} else if at, ok := rawUpstream["apiKey"].(string); ok && at != "" {
			authToken = at
		}
	}

	status := strings.ToLower(strings.TrimSpace(upstream.Status))
	if status == "success" || status == "ok" {
		status = "authorized"
	}
	if status == "" {
		if authToken != "" {
			status = "authorized"
		} else if strings.Contains(strings.ToLower(upstream.Error), "expired") || strings.Contains(strings.ToLower(upstream.Message), "expired") {
			status = "expired"
		} else {
			status = "pending"
		}
	}

	email := upstream.Email
	name := upstream.Name
	userId := ""
	if rawUpstream != nil {
		if email == "" {
			if e, ok := rawUpstream["email"].(string); ok {
				email = e
			}
		}
		if name == "" {
			if n, ok := rawUpstream["name"].(string); ok {
				name = n
			}
		}
		if uid, ok := rawUpstream["userId"].(string); ok {
			userId = uid
		} else if uid, ok := rawUpstream["user_id"].(string); ok {
			userId = uid
		}
		if userObj, ok := rawUpstream["user"].(map[string]any); ok {
			if email == "" {
				if e, ok := userObj["email"].(string); ok {
					email = e
				}
			}
			if name == "" {
				if n, ok := userObj["name"].(string); ok {
					name = n
				}
			}
			if userId == "" {
				if uid, ok := userObj["id"].(string); ok {
					userId = uid
				}
			}
		}
	}

	if status == "pending" {
		handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
			"status": "pending",
		})
		return
	}

	if status == "expired" {
		freebuffPendingSessions.Delete(fpID)
		handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
			"status": "expired",
		})
		return
	}

	if (status == "authorized" || authToken != "") && authToken != "" {
		freebuffPendingSessions.Delete(fpID)
		connID := "fb-" + shortHash(authToken)
		connName := name
		if connName == "" && email != "" {
			connName = "Freebuff (" + email + ")"
		} else if connName == "" {
			connName = "Freebuff (" + connID + ")"
		}

		dataMap := map[string]any{
			"authToken":   authToken,
			"apiKey":      authToken,
			"accessToken": authToken,
		}
		if email != "" {
			dataMap["email"] = email
		}
		if name != "" {
			dataMap["name"] = name
		}
		if userId != "" {
			dataMap["userId"] = userId
		}

		dataBytes, err := json.Marshal(dataMap)
		if err != nil {
			handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to marshal connection data")
			return
		}

		log.Info("oauth", "saving freebuff connection", "connID", connID, "name", connName, "email", email)

		if h.Repo != nil && h.Repo.RawDB() != nil {
			now := currentTimestamp()
			var exists int
			_ = h.Repo.RawDB().QueryRow("SELECT COUNT(*) FROM providerConnections WHERE id = ?", connID).Scan(&exists)
			if exists > 0 {
				_, err = h.Repo.RawDB().Exec(
					"UPDATE providerConnections SET name = ?, data = ?, updatedAt = ? WHERE id = ?",
					connName, string(dataBytes), now, connID,
				)
			} else {
				_, err = h.Repo.RawDB().Exec(
					"INSERT INTO providerConnections (id, provider, authType, name, isActive, data, createdAt, updatedAt) VALUES (?, 'freebuff', 'oauth', ?, 1, ?, ?, ?)",
					connID, connName, string(dataBytes), now, now,
				)
			}
			if err != nil {
				log.Error("oauth", "save freebuff connection failed", "conn", connID, "error", err)
				handlerutil.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to save connection: %v", err))
				return
			}
		}

		handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
			"status":       "authorized",
			"connectionId": connID,
		})
		return
	}

	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"status": status,
	})
}
