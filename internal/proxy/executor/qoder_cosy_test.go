package executor

import (
	"strings"
	"testing"
)

// The chat request signs with sigPath "/api/v2/service/pro/sse/agent_chat_generation"
// (leading "/algo" stripped) — this used to be hardcoded and must stay identical
// now that it is derived from the request URL.
func TestQoderCosySigPath(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		{"https://api3.qoder.sh/algo/api/v2/service/pro/sse/agent_chat_generation", "/api/v2/service/pro/sse/agent_chat_generation"},
		{"https://api2.qoder.sh/algo/api/v2/model/list", "/api/v2/model/list"},
		{"https://api3.qoder.sh/api/v2/model/list", "/api/v2/model/list"},
	}
	for _, tc := range cases {
		if got := qoderCosySigPath(tc.url); got != tc.want {
			t.Errorf("qoderCosySigPath(%s) = %q, want %q", tc.url, got, tc.want)
		}
	}
}

func TestBuildQoderCosyHeaders_SignsRequestedPath(t *testing.T) {
	const modelListURL = "https://api2.qoder.sh/algo/api/v2/model/list"
	headers, err := BuildQoderCosyHeaders(nil, modelListURL, "user-1", "jt-abc")
	if err != nil {
		t.Fatalf("BuildQoderCosyHeaders: %v", err)
	}
	if got := headers["Cosy-Sigpath"]; got != "/api/v2/model/list" {
		t.Errorf("Cosy-Sigpath = %q, want /api/v2/model/list", got)
	}
	if got := headers["Cosy-User"]; got != "user-1" {
		t.Errorf("Cosy-User = %q, want user-1", got)
	}
	if got := headers["Authorization"]; !strings.HasPrefix(got, "Bearer COSY.") {
		t.Errorf("Authorization = %q, want a COSY bearer header", got)
	}
	if headers["Cosy-Key"] == "" || headers["Cosy-Date"] == "" {
		t.Errorf("missing COSY key/date headers: %v", headers)
	}
}
