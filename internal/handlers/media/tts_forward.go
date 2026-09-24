package media

import (
	"bytes"
	"encoding/base64"
	json "encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"9router/proxy/internal/handlers/chat"
	"9router/proxy/internal/handlerutil"
	"9router/proxy/internal/log"
	"9router/proxy/internal/usagetracker"
)

func (h *MediaHandler) forwardTTSRequest(w http.ResponseWriter, r *http.Request, body []byte) {
	var req struct {
		Model          string `json:"model"`
		Input          string `json:"input"`
		Voice          string `json:"voice"`
		ResponseFormat string `json:"response_format"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.Input) == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "Missing required field: input")
		return
	}

	modelInfo, err := h.ChatH.ResolveModel(req.Model)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 1. edge-tts
	if modelInfo.Provider == "edge-tts" {
		voice := req.Voice
		if voice == "" || voice == "alloy" || voice == "default" {
			voice = strings.Split(modelInfo.Model, "/")[0]
		}
		if voice == "" || voice == "edge-tts" {
			voice = "en-US-AriaNeural"
		}
		audio, err := SynthesizeEdgeTTS(r.Context(), h.Client, req.Input, voice)
		if err != nil {
			log.Error("media", "edge-tts synthesis failed", "error", err)
			handlerutil.WriteJSONError(w, http.StatusBadGateway, err.Error())
			return
		}
		h.writeTTSResponse(w, r, req.ResponseFormat, audio, "audio/mpeg", "mp3")
		h.trackTTSUsage(modelInfo.Provider, modelInfo.Model)
		return
	}

	// 2. google-tts
	if modelInfo.Provider == "google-tts" {
		lang := req.Voice
		if lang == "" || lang == "alloy" || lang == "default" {
			lang = strings.Split(modelInfo.Model, "/")[0]
		}
		if lang == "" || lang == "google-tts" {
			lang = "en"
		}
		audio, err := SynthesizeGoogleTTS(r.Context(), h.Client, req.Input, lang)
		if err != nil {
			log.Error("media", "google-tts synthesis failed", "error", err)
			handlerutil.WriteJSONError(w, http.StatusBadGateway, err.Error())
			return
		}
		h.writeTTSResponse(w, r, req.ResponseFormat, audio, "audio/mpeg", "mp3")
		h.trackTTSUsage(modelInfo.Provider, modelInfo.Model)
		return
	}

	// 3. nvidia
	if modelInfo.Provider == "nvidia" {
		h.forwardNvidiaTTS(w, r, body, modelInfo, req.Input, req.Voice, req.ResponseFormat)
		return
	}

	// 4. Default OpenAI-compatible TTS
	h.forwardMediaRequest(w, r, body, "tts-1", "/v1/audio/speech")
}

func (h *MediaHandler) trackTTSUsage(provider, model string) {
	usagetracker.GetTracker().PushRecent(usagetracker.RecentRequest{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Model:     model,
		Provider:  provider,
		Status:    "ok",
	}, h.Repo)
}

func (h *MediaHandler) writeTTSResponse(w http.ResponseWriter, r *http.Request, reqFormat string, audio []byte, mimeType, ext string) {
	isJSON := reqFormat == "json" || r.URL.Query().Get("response_format") == "json"
	if isJSON {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		jsonBytes, _ := json.Marshal(map[string]any{
			"format": ext,
			"audio":  base64.StdEncoding.EncodeToString(audio),
		})
		_, _ = w.Write(jsonBytes)
		return
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Length", strconv.Itoa(len(audio)))
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"speech.%s\"", ext))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(audio)
}

func (h *MediaHandler) forwardNvidiaTTS(w http.ResponseWriter, r *http.Request, body []byte, modelInfo *chat.ModelInfo, input, voice, respFmt string) {
	preferredConnID := r.Header.Get("x-connection-id")
	if preferredConnID == "" {
		preferredConnID = r.Header.Get("x-provider-connection-id")
	}
	if preferredConnID == "" {
		preferredConnID = modelInfo.ConnectionID
	}
	conn, connData, err := h.ChatH.GetBestConnection("nvidia", preferredConnID, nil, modelInfo.Model)
	if err != nil || connData == nil {
		handlerutil.WriteJSONError(w, http.StatusNotFound, "no active connections for provider: nvidia")
		return
	}
	apiKey := chat.ExtractAPIKey(connData)
	if apiKey == "" {
		handlerutil.WriteJSONError(w, http.StatusUnauthorized, "no API key found for nvidia")
		return
	}

	modelID := strings.Split(modelInfo.Model, "/")[0]
	if modelID == "" {
		modelID = "fastpitch"
	}
	voiceID := voice
	if voiceID == "" || voiceID == "alloy" {
		parts := strings.Split(modelInfo.Model, "/")
		if len(parts) > 1 {
			voiceID = parts[1]
		} else {
			voiceID = "default"
		}
	}

	payload := map[string]any{
		"input": map[string]string{
			"text": input,
		},
		"voice": voiceID,
		"model": modelID,
	}
	payloadBytes, _ := json.Marshal(payload)

	targetURL := "https://integrate.api.nvidia.com/v1/audio/speech"
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, targetURL, bytes.NewReader(payloadBytes))
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to create nvidia tts request")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := h.ChatH.GetClientForConnection(connData)
	resp, err := client.Do(req)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadGateway, "nvidia tts request failed: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBytes, _ := io.ReadAll(resp.Body)
		log.Warn("media", "nvidia tts upstream error", "status", resp.StatusCode, "body", string(errBytes))
		handlerutil.WriteJSONError(w, resp.StatusCode, string(errBytes))
		return
	}

	audioBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadGateway, "failed to read nvidia audio response")
		return
	}

	if conn != nil {
		h.Repo.UpdateConnectionLastUsed(conn.ID)
	}
	h.trackTTSUsage("nvidia", modelInfo.Model)
	h.writeTTSResponse(w, r, respFmt, audioBytes, "audio/wav", "wav")
}
