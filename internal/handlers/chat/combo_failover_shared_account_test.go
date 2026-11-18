package chat

import (
	json "encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"9router/proxy/internal/db"
)

// setupSharedAccountCombo registers ONE connection for provider deepseek
// whose upstream fails requests for deepseek-chat with failStatus/failBody
// and serves deepseek-reasoner normally, plus a round-robin combo over both
// models on that single shared account. This mirrors an aggregator node
// (one API key, many models behind it) where a per-model failure must not
// take the remaining models out of the rotation.
func setupSharedAccountCombo(t *testing.T, failStatus int, failBody string) (*ChatHandler, *atomic.Int32, func()) {
	return setupSharedAccountComboStrategy(t, failStatus, failBody, "round-robin")
}

// setupSharedAccountComboStrategy is setupSharedAccountCombo with an explicit
// combo strategy, so a test can pin the strategy the stored combo actually uses.
func setupSharedAccountComboStrategy(t *testing.T, failStatus int, failBody, strategy string) (*ChatHandler, *atomic.Int32, func()) {
	t.Helper()
	database, cleanupDB := setupChatTestDB(t)

	var failHits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, 0, 4096)
		buf := make([]byte, 4096)
		for {
			n, err := r.Body.Read(buf)
			body = append(body, buf[:n]...)
			if err != nil {
				break
			}
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(string(body), `"deepseek-chat"`) {
			failHits.Add(1)
			w.WriteHeader(failStatus)
			w.Write([]byte(failBody))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"chatcmpl-ok","object":"chat.completion","created":1,"model":"deepseek-reasoner","choices":[{"index":0,"message":{"role":"assistant","content":"hello from reasoner"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`))
	}))

	if _, err := database.Exec(`DELETE FROM providerConnections WHERE id IN ('conn-1', 'conn-2')`); err != nil {
		t.Fatalf("clear seeded connections: %v", err)
	}
	data, _ := json.Marshal(map[string]any{"apiKey": "sk-mock-key", "baseUrl": upstream.URL})
	if _, err := database.Exec(`INSERT INTO providerConnections (id, provider, authType, name, priority, isActive, data, createdAt, updatedAt) VALUES
		('conn-shared', 'deepseek', 'apikey', 'Shared Mock', 0, 1, ?, '2026-07-18T00:00:00Z', '2026-07-18T00:00:00Z')`,
		string(data)); err != nil {
		t.Fatalf("insert shared connection: %v", err)
	}

	comboModels, _ := json.Marshal([]string{"deepseek/deepseek-chat", "deepseek/deepseek-reasoner"})
	if _, err := database.Exec(`INSERT INTO combos (id, name, kind, models, createdAt, updatedAt) VALUES
		('combo-failover', 'combo-failover', 'fallback', ?, '2026-07-18T00:00:00Z', '2026-07-18T00:00:00Z')`,
		string(comboModels)); err != nil {
		t.Fatalf("insert combo: %v", err)
	}
	if _, err := database.Exec(`INSERT INTO settings (id, data) VALUES (1, ?) ON CONFLICT(id) DO UPDATE SET data = excluded.data`,
		`{"comboStrategies":{"combo-failover":{"fallbackStrategy":`+strconv.Quote(strategy)+`}}}`); err != nil {
		t.Fatalf("seed settings: %v", err)
	}

	return NewChatHandler(db.NewRepo(database)), &failHits, func() {
		upstream.Close()
		cleanupDB()
	}
}

// A combo member that fails must hand the turn to the next member after ONE
// attempt on the shared connection, whatever the failure status. Before the
// fix, a 500/501 was not in RetryableStatusCodes, so the loop re-picked the
// same sole connection up to 10 times per member; and a generic 401 wrote an
// account-wide rate-limit lock that silently starved every later member on
// the shared account, so the client kept the first model's error.
func TestComboFailoverSharedAccount(t *testing.T) {
	tests := []struct {
		name       string
		failStatus int
		failBody   string
	}{
		{
			name:       "501 advances to the next member after one attempt",
			failStatus: http.StatusNotImplemented,
			failBody:   `{"error":{"message":"not implemented for this model","type":"server_error","code":501}}`,
		},
		{
			name:       "500 advances to the next member after one attempt",
			failStatus: http.StatusInternalServerError,
			failBody:   `{"error":{"message":"internal error","type":"server_error","code":500}}`,
		},
		{
			name:       "generic 401 does not poison the shared account for the next member",
			failStatus: http.StatusUnauthorized,
			failBody:   `{"error":{"message":"unauthorized","type":"auth_error","code":401}}`,
		},
		{
			name:       "402 payment required advances to the next member after one attempt",
			failStatus: http.StatusPaymentRequired,
			failBody:   `{"error":{"message":"insufficient funds for this paid model","type":"payment_error","code":402}}`,
		},
		{
			name:       "400 insufficient credits advances to the next member after one attempt",
			failStatus: http.StatusBadRequest,
			failBody:   `{"error":{"message":"Insufficient credits to complete this request","type":"invalid_request_error","code":400}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, failHits, cleanup := setupSharedAccountCombo(t, tt.failStatus, tt.failBody)
			defer cleanup()

			req := httptest.NewRequest("POST", "/v1/chat/completions",
				strings.NewReader(`{"model":"combo-failover","messages":[{"role":"user","content":"hi"}],"stream":false}`))
			rec := httptest.NewRecorder()
			handler.HandleChatCompletions(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 from the second combo member; body: %s", rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "hello from reasoner") {
				t.Fatalf("body does not carry the second member's answer: %s", rec.Body.String())
			}
			if got := failHits.Load(); got != 1 {
				t.Errorf("failing member was attempted %d times, want exactly 1 before advancing", got)
			}
		})
	}
}

// A 400 credit-balance fault is request-scoped (providers says no cooldown
// applies to it on its own), but within a combo it means this model's balance
// is spent. The combo must record a cooldown for it so a LATER turn does not
// re-select the dead member — not just advance past it this turn.
func TestComboCreditsErrorCoolsModelAndAdvances(t *testing.T) {
	handler, failHits, cleanup := setupSharedAccountComboStrategy(t,
		http.StatusBadRequest,
		`{"error":{"message":"insufficient credit balance","type":"invalid_request_error","code":400}}`,
		"fallback")
	defer cleanup()

	// First request: the failing (first) member is attempted once, its 400
	// credits fault cools the model, and the combo answers from the second.
	req1 := httptest.NewRequest("POST", "/v1/chat/completions",
		strings.NewReader(`{"model":"combo-failover","messages":[{"role":"user","content":"hi"}],"stream":false}`))
	rec1 := httptest.NewRecorder()
	handler.HandleChatCompletions(rec1, req1)
	if rec1.Code != http.StatusOK || !strings.Contains(rec1.Body.String(), "hello from reasoner") {
		t.Fatalf("first request: status = %d body = %s, want 200 from reasoner", rec1.Code, rec1.Body.String())
	}
	if got := failHits.Load(); got != 1 {
		t.Fatalf("first request: failing member attempted %d times, want 1", got)
	}

	// Second request: "fallback" keeps the stored order, so the failing member
	// is first again. Its cooldown must skip it before any dial — failHits stays
	// 1 and the request still answers from the healthy member.
	req2 := httptest.NewRequest("POST", "/v1/chat/completions",
		strings.NewReader(`{"model":"combo-failover","messages":[{"role":"user","content":"hi again"}],"stream":false}`))
	rec2 := httptest.NewRecorder()
	handler.HandleChatCompletions(rec2, req2)
	if rec2.Code != http.StatusOK || !strings.Contains(rec2.Body.String(), "hello from reasoner") {
		t.Fatalf("second request: status = %d body = %s, want 200 from reasoner", rec2.Code, rec2.Body.String())
	}
	if got := failHits.Load(); got != 1 {
		t.Errorf("second request re-hit the cooled model: failHits = %d, want it to stay at 1", got)
	}
}
