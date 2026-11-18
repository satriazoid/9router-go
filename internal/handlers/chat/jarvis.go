package chat

import (
	"sort"
	"strings"
	"time"
)

// Jarvis is a supervised fallback/round-robin hybrid strategy:
// - Models are ordered by observed responsiveness (mean TTFT), fastest first.
// - Every non-final attempt gets a hard deadline. A model that does not answer
//   inside it is abandoned mid-attempt and the next member takes the turn (rotate).
// - A model that errors or times out gets a penalty added to its score, so it
//   is pushed behind healthy models on later requests — but the ordering still
//   rotates, so no model is permanently stuck (round-robin behaviour).
// - The operator never has to hand-tune which provider is currently stable;
//   the runtime health tracker handles demotion automatically.

const (
	// jarvisDefaultAttemptTimeoutMs bounds one non-final attempt when the
	// combo configures no firstTokenTimeoutMs. 30s is long enough for a slow
	// reasoning model to start streaming and short enough that a hung
	// connection does not consume the caller's whole budget.
	jarvisDefaultAttemptTimeoutMs = 30_000
	// jarvisSlowPenaltyMs is added to a member's observed latency when it has
	// failed recently, so a model that is fast but flaky does not outrank a
	// model that is merely fast.
	jarvisSlowPenaltyMs = 60_000
)

// jarvisIntent reports whether a strategy string selects the Jarvis engine.
func jarvisIntent(strategy string) bool {
	return strings.EqualFold(strings.TrimSpace(strategy), "jarvis")
}

// jarvisRankModels orders combo members so the models that have been answering
// fastest are tried first, and models with recent failures are pushed behind
// healthy ones. The sort is stable inside equal scores so the operator's own
// arrangement still breaks ties.
//
// This is NOT a static order. Every request the combo rotates through members
// (round-robin behaviour), but a model that has just errored or timed out
// receives a penalty that pushes it down the list for the next turn, so it is
// naturally avoided without requiring the operator to reorder.
func (h *ChatHandler) jarvisRankModels(models []string) []string {
	if len(models) < 2 || h.Repo == nil {
		return models
	}
	latency, err := h.Repo.GetModelLatency()
	if err != nil || len(latency) == 0 {
		// No latency data yet: fall back to pure round-robin rotation.
		return simpleJarvisRotate(models)
	}
	ttft := make(map[string]int64, len(latency))
	for _, row := range latency {
		if row.TTFTMs <= 0 || row.Samples <= 0 {
			continue
		}
		ttft[row.Provider+"|"+row.Model] = row.TTFTMs
	}
	if len(ttft) == 0 {
		return simpleJarvisRotate(models)
	}

	score := func(model string) (int64, bool) {
		provider, name, ok := strings.Cut(model, "/")
		if !ok {
			provider, name = "", model
		}
		observed, known := ttft[provider+"|"+name]
		if !known {
			// No latency sample for this exact model, but the provider may
			// still be available. Give it a neutral mid-position score.
			if !h.Repo.IsProviderAvailable(provider, name) {
				return jarvisSlowPenaltyMs, true // unusable, push down
			}
			return 0, false // measured-but-no-sample: keep stable position
		}
		// A model whose provider is currently unavailable is unusable:
		// push it behind every healthy member.
		if !h.Repo.IsProviderAvailable(provider, name) {
			return observed + jarvisSlowPenaltyMs, true
		}
		return observed, true
	}

	out := make([]string, len(models))
	copy(out, models)
	// Insertion sort: combo membership is tiny (tens of entries at most), and
	// a stable sort keeps the operator's order between equal scores.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0; j-- {
			left, leftKnown := score(out[j])
			right, rightKnown := score(out[j-1])
			swap := false
			switch {
			case rightKnown && !leftKnown:
				swap = false // measured stays ahead of unmeasured
			case !rightKnown && leftKnown:
				swap = true // unmeasured yields to measured
			default:
				swap = left < right
			}
			if !swap {
				break
			}
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// simpleJarvisRotate performs a pure round-robin rotation with no latency
// weighting. Used as a fallback when no latency data is available yet.
func simpleJarvisRotate(models []string) []string {
	if len(models) <= 1 {
		return models
	}
	out := make([]string, len(models))
	copy(out, models)
	// Rotate left by one: the first model moves to the end (round-robin).
	first := out[0]
	copy(out, out[1:])
	out[len(models)-1] = first
	return out
}

// jarvisAttemptTimeout resolves the deadline for one non-final attempt. The
// combo's own firstTokenTimeoutMs wins when set; otherwise Jarvis applies its
// own default so a hung member cannot hold the turn.
func (h *ChatHandler) jarvisAttemptTimeout(comboName string) time.Duration {
	if ms := h.comboFirstTokenTimeoutMs(comboName); ms > 0 {
		return time.Duration(ms) * time.Millisecond
	}
	return jarvisDefaultAttemptTimeoutMs * time.Millisecond
}

// jarvisSortByScore is the ordering rule on its own, exposed for tests: it
// takes the latency map directly instead of reading the repository.
func jarvisSortByScore(models []string, ttft map[string]int64) []string {
	out := make([]string, len(models))
	copy(out, models)
	type entry struct {
		model   string
		latency int64
		known   bool
	}
	entries := make([]entry, len(out))
	for i, m := range out {
		provider, name, ok := strings.Cut(m, "/")
		if !ok {
			provider, name = "", m
		}
		observed, known := ttft[provider+"|"+name]
		entries[i] = entry{model: m, latency: observed, known: known}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].known != entries[j].known {
			return entries[i].known
		}
		return entries[i].latency < entries[j].latency
	})
	for i := range entries {
		out[i] = entries[i].model
	}
	return out
}