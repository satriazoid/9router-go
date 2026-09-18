package dashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"9router/proxy/internal/handlerutil"
	"9router/proxy/internal/handlers/chat"
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

	// Antigravity quota resolution
	if conn.Provider == "antigravity" && data != nil {
		accessToken, _ := data["accessToken"].(string)
		projectID, _ := data["projectId"].(string)
		if accessToken != "" {
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			defer cancel()

			quotas, qErr := chat.RefreshAntigravityQuota(ctx, http.DefaultClient, conn.ID, accessToken, projectID)
			if qErr == nil && len(quotas) > 0 {
				respQuotas := make(map[string]any)
				for m, q := range quotas {
					respQuotas[m] = map[string]any{
						"used":                100 - q.RemainingPercentage,
						"total":               100,
						"remainingPercentage": q.RemainingPercentage,
						"resetAt":             q.ResetAt.Format(time.RFC3339),
						"unlimited":           false,
						"displayName":         m,
					}
				}
				handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
					"plan":   "Antigravity",
					"quotas": respQuotas,
				})
				return
			}
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
