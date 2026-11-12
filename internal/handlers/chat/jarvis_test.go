package chat

import "testing"

// A jarvis combo must never pin one member. Every new turn advances the
// rotation pointer, so N consecutive turns visit N distinct leads even when the
// repository carries no latency samples at all.
func TestApplyComboStrategy_jarvisRotates(t *testing.T) {
	h := NewChatHandler(nil)
	models := []string{"a/x", "b/y", "c/z"}

	seen := map[string]bool{}
	for i := 0; i < len(models); i++ {
		lead := h.applyComboStrategy("jarvis", models, "comboJ", 1, true)[0]
		seen[lead] = true
	}
	if len(seen) != len(models) {
		t.Fatalf("jarvis stuck: %d distinct leads over %d turns (%v)", len(seen), len(models), seen)
	}
}

// Mid-turn requests must stay on the model serving the turn, otherwise a
// tool-use sequence would switch providers between calls.
func TestApplyComboStrategy_jarvisStickyMidTurn(t *testing.T) {
	h := NewChatHandler(nil)
	models := []string{"a/x", "b/y", "c/z"}

	lead := h.applyComboStrategy("jarvis", models, "comboJ2", 1, true)[0]
	for i := 0; i < 3; i++ {
		if got := h.applyComboStrategy("jarvis", models, "comboJ2", 1, false)[0]; got != lead {
			t.Fatalf("mid-turn lead = %s, want %s", got, lead)
		}
	}
}

// Ranking puts models with latency samples ahead of unmeasured ones and orders
// measured models fastest-first.
func TestJarvisSortByScore(t *testing.T) {
	models := []string{"a/slow", "b/fast", "c/none"}
	got := jarvisSortByScore(models, map[string]int64{"a|slow": 900, "b|fast": 120})
	want := []string{"b/fast", "a/slow", "c/none"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("rank = %v, want %v", got, want)
		}
	}
}
