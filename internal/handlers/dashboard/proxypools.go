package dashboard

import (
	json "encoding/json/v2"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"9router/proxy/internal/db"
	"9router/proxy/internal/handlerutil"
)

// HandleGetProxyPools handles GET /api/proxy-pools.
func (h *DashboardHandler) HandleGetProxyPools(w http.ResponseWriter, r *http.Request) {
	includeUsage := r.URL.Query().Get("includeUsage") == "true"
	pools, err := h.Repo.ListProxyPools()
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if includeUsage {
		conns, _ := h.Repo.GetProviderConnections("", false)
		boundCounts := make(map[string]int)
		for _, c := range conns {
			if c == nil {
				continue
			}
			var dataMap map[string]any
			if c.Data != "" {
				_ = json.Unmarshal([]byte(c.Data), &dataMap)
			}
			poolID := handlerutil.GetString(dataMap, "proxyPoolId")
			if poolID == "" {
				if psd, ok := dataMap["providerSpecificData"].(map[string]any); ok {
					poolID = handlerutil.GetString(psd, "proxyPoolId")
				}
			}
			if poolID != "" {
				boundCounts[poolID]++
			}
		}

		for _, p := range pools {
			if id, ok := p["id"].(string); ok {
				p["boundConnectionCount"] = boundCounts[id]
			}
		}
	}

	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"proxyPools": pools,
	})
}

// HandleCreateProxyPool handles POST /api/proxy-pools.
func (h *DashboardHandler) HandleCreateProxyPool(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	defer r.Body.Close()

	var req struct {
		Name        string   `json:"name"`
		ProxyURL    string   `json:"proxyUrl"`
		URLs        []string `json:"urls"`
		Type        string   `json:"type"`
		NoProxy     string   `json:"noProxy"`
		StrictProxy bool     `json:"strictProxy"`
		IsActive    bool     `json:"isActive"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if req.Name == "" {
		req.Name = "Proxy Pool"
	}
	if req.Type == "" {
		if strings.HasPrefix(req.ProxyURL, "socks") {
			req.Type = "socks5"
		} else {
			req.Type = "http"
		}
	}

	poolData := db.ProxyPoolData{
		Name:        req.Name,
		ProxyURL:    req.ProxyURL,
		NoProxy:     req.NoProxy,
		Type:        req.Type,
		StrictProxy: req.StrictProxy,
	}

	created, err := h.Repo.InsertProxyPool(poolData)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	handlerutil.WriteJSON(w, http.StatusOK, created)
}

// HandleUpdateProxyPool handles PUT /api/proxy-pools/{id}.
func (h *DashboardHandler) HandleUpdateProxyPool(w http.ResponseWriter, r *http.Request) {
	id := getURLParam(r, "id")
	if id == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "missing proxy pool id")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	defer r.Body.Close()

	var updates map[string]any
	if err := json.Unmarshal(body, &updates); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if err := h.Repo.UpdateProxyPool(id, updates); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"success": true})
}

// HandleDeleteProxyPool handles DELETE /api/proxy-pools/{id}.
func (h *DashboardHandler) HandleDeleteProxyPool(w http.ResponseWriter, r *http.Request) {
	id := getURLParam(r, "id")
	if id == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "missing proxy pool id")
		return
	}

	if err := h.Repo.DeleteProxyPool(id); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"success": true})
}

// HandleTestProxyPool handles POST /api/proxy-pools/{id}/test.
func (h *DashboardHandler) HandleTestProxyPool(w http.ResponseWriter, r *http.Request) {
	id := getURLParam(r, "id")
	if id == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "missing proxy pool id")
		return
	}

	pool, err := h.Repo.GetProxyPool(id)
	if err != nil || pool == nil {
		handlerutil.WriteJSONError(w, http.StatusNotFound, "proxy pool not found")
		return
	}

	targetURL := pool.NextURL()
	if targetURL == "" {
		_ = h.Repo.SetProxyPoolStatus(id, "failed", 0)
		handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
			"success": false,
			"error":   "no proxy URLs configured",
		})
		return
	}

	start := time.Now()
	proxyParsed, err := url.Parse(targetURL)
	if err != nil {
		_ = h.Repo.SetProxyPoolStatus(id, "failed", 0)
		handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
			"success": false,
			"error":   "invalid proxy URL format",
		})
		return
	}

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyParsed),
		},
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get("https://www.google.com/generate_204")
	latencyMs := time.Since(start).Milliseconds()

	if err != nil || (resp != nil && resp.StatusCode >= 400) {
		status := "failed"
		_ = h.Repo.SetProxyPoolStatus(id, status, latencyMs)
		errStr := "connection timed out or failed"
		if err != nil {
			errStr = err.Error()
		}
		handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
			"success": false,
			"status":  status,
			"latency": latencyMs,
			"error":   errStr,
		})
		return
	}
	defer resp.Body.Close()

	_ = h.Repo.SetProxyPoolStatus(id, "passed", latencyMs)
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"status":  "passed",
		"latency": latencyMs,
	})
}
