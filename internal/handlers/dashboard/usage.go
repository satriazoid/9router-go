package dashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"9router/proxy/internal/handlerutil"
)

// HandleGetConnectionUsage handles GET /api/usage/{connectionId}
func (h *DashboardHandler) HandleGetConnectionUsage(w http.ResponseWriter, r *http.Request) {
	connID := getURLParam(r, "connectionId")
	if connID == "" {
		connID = getURLParam(r, "id")
	}
	if connID == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "missing connectionId")
		return
	}

	conn, err := h.Repo.GetProviderConnectionByID(connID)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if conn == nil {
		handlerutil.WriteJSONError(w, http.StatusNotFound, "connection not found")
		return
	}

	var data map[string]any
	if conn.Data != "" {
		_ = json.Unmarshal([]byte(conn.Data), &data)
	}

	// Live provider quota fetchers (ports of open-sse/services/usage/*.js).
	// Antigravity keeps its existing dedicated path below.
	if res, ok := fetchProviderUsage(r.Context(), conn.Provider, data); ok {
		handlerutil.WriteJSON(w, http.StatusOK, res.toResponse())
		return
	}

	// Antigravity quota resolution (dashboard presentation with tier check +
	// weekly overlay — parity with open-sse/services/usage/google.js).
	if conn.Provider == "antigravity" && data != nil {
		accessToken, _ := data["accessToken"].(string)
		projectID, _ := data["projectId"].(string)
		if accessToken != "" {
			ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
			defer cancel()

			res := fetchAntigravityDashboardUsage(ctx, accessToken, projectID)
			handlerutil.WriteJSON(w, http.StatusOK, res.toResponse())
			return
		}
	}

	// Fallback for connections with rate limit or locks in data
	respQuotas := make(map[string]any)
	if data != nil {
		if rateLimitedUntil, ok := data["rateLimitedUntil"].(string); ok && rateLimitedUntil != "" {
			respQuotas["default"] = map[string]any{
				"remainingPercentage": 0,
				"resetAt":             rateLimitedUntil,
				"displayName":         conn.Provider,
			}
		}
		for k, v := range data {
			if len(k) > 10 && k[:10] == "modelLock_" {
				model := k[10:]
				if lockStr, ok := v.(string); ok && lockStr != "" {
					respQuotas[model] = map[string]any{
						"remainingPercentage": 0,
						"resetAt":             lockStr,
						"displayName":         model,
					}
				}
			}
		}
	}

	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"plan":   conn.Provider,
		"quotas": respQuotas,
	})
}

// HandleGetUsageProviders handles GET /api/usage/providers
func (h *DashboardHandler) HandleGetUsageProviders(w http.ResponseWriter, r *http.Request) {
	conns, err := h.Repo.GetProviderConnections("", false)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	seen := make(map[string]bool)
	type ProviderItem struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	var list []ProviderItem
	for _, c := range conns {
		if c.Provider != "" && !seen[c.Provider] {
			seen[c.Provider] = true
			list = append(list, ProviderItem{
				ID:   c.Provider,
				Name: c.Provider,
			})
		}
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"providers": list,
	})
}
