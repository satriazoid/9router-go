package executor

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"

	"9router/proxy/internal/log"
	"9router/proxy/internal/proxy"
	"9router/proxy/internal/translator"
)

// handleClaudeMessagesStream pipes a Claude Messages SSE stream from upstream,
// translating chunks to OpenAI SSE format when the downstream client is an OpenAI client.
func handleClaudeMessagesStream(w http.ResponseWriter, req *Request, upstream io.Reader) error {
	if req.TranslateResp {
		// Client requested Claude format (/v1/messages) and upstream is already Claude format.
		// Stream directly via SSECopy without format translation.
		startTime := req.StartTime
		if startTime.IsZero() {
			startTime = time.Now()
		}
		return sseStream(w, upstream, false, startTime, req.TTFT, req.ResponseBuf, req.Ctx)
	}

	// Client requested OpenAI format (/v1/chat/completions) but upstream is Claude Messages SSE.
	// Translate each Claude SSE event into standard OpenAI chunk SSE (choices[0].delta).
	hw := proxy.NewHeartbeatWriter(req.Ctx, w, 0)
	defer hw.Close()
	flusher := proxy.WriteSSEHeaders(hw)

	state := &translator.ClaudeToOpenAIStreamState{}
	doneSeen := false

	err := proxy.ScanStream(upstream, func(payload []byte) {
		if doneSeen {
			return
		}
		trimmed := bytes.TrimSpace(payload)
		if string(trimmed) == "[DONE]" {
			doneSeen = true
			_, _ = hw.Write([]byte("data: [DONE]\n\n"))
			if flusher != nil {
				flusher.Flush()
			}
			return
		}

		out, terr := translator.TranslateClaudeChunkToOpenAI(payload, state)
		if terr != nil {
			log.Error("executor", "translate claude chunk to openai", "error", terr)
			return
		}
		if len(out) == 0 {
			return
		}

		if req.TTFT != nil && *req.TTFT == 0 {
			startTime := req.StartTime
			if startTime.IsZero() {
				startTime = time.Now()
			}
			*req.TTFT = time.Since(startTime).Milliseconds()
		}
		if req.ResponseBuf != nil {
			req.ResponseBuf.Write(out)
		}
		if _, werr := hw.Write(out); werr != nil {
			return
		}
		if flusher != nil {
			flusher.Flush()
		}
		if bytes.Contains(out, []byte("[DONE]")) {
			doneSeen = true
		}
	})

	if !doneSeen {
		_, _ = hw.Write([]byte("data: [DONE]\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}
	return err
}

// handleClaudeMessagesNonStream handles non-streaming Claude Messages responses,
// translating Claude JSON to OpenAI JSON when the downstream client is an OpenAI client.
func handleClaudeMessagesNonStream(w http.ResponseWriter, req *Request, upstream io.Reader) error {
	body, err := io.ReadAll(io.LimitReader(upstream, 10*1024*1024))
	if err != nil {
		return fmt.Errorf("read claude response body: %w", err)
	}

	if req.TranslateResp {
		// Client requested Claude format, upstream is Claude format: pass through
		return jsonResponse(req.Ctx, w, bytes.NewReader(body), false, req.ResponseBuf)
	}

	// Client requested OpenAI format, upstream is Claude format: translate!
	converted, err := translator.TranslateClaudeResponseToOpenAI(body)
	if err != nil {
		return fmt.Errorf("translate claude response to openai: %w", err)
	}
	return jsonResponse(req.Ctx, w, bytes.NewReader(converted), false, req.ResponseBuf)
}
