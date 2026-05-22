package providers

import (
	"net/http"
	"net/url"
	"testing"
	"time"
)

func writeAnthropicFixture(t *testing.T, w http.ResponseWriter, r *http.Request, usage, cost string) {
	t.Helper()
	switch r.URL.Path {
	case anthropicUsagePath:
		_, _ = w.Write([]byte(usage))
	case anthropicCostPath:
		_, _ = w.Write([]byte(cost))
	default:
		t.Fatalf("unexpected path %q", r.URL.Path)
	}
}

func assertAnthropicBaseQuery(t *testing.T, name string, q url.Values) {
	t.Helper()
	if q.Get("starting_at") == "" || q.Get("ending_at") == "" {
		t.Fatalf("%s: missing time query: %v", name, q)
	}
	if _, err := time.Parse(time.RFC3339, q.Get("starting_at")); err != nil {
		t.Errorf("%s: bad starting_at: %v", name, err)
	}
	if got := q.Get("bucket_width"); got != "1d" {
		t.Errorf("%s: bucket_width = %q", name, got)
	}
	if got := q.Get("limit"); got != "31" {
		t.Errorf("%s: limit = %q, want 31", name, got)
	}
}

func assertGroups(t *testing.T, name string, q url.Values, want []string) {
	t.Helper()
	got := map[string]bool{}
	for _, g := range q["group_by[]"] {
		got[g] = true
	}
	for _, g := range want {
		if !got[g] {
			t.Errorf("%s: missing group_by[]=%s; got=%v", name, g, q["group_by[]"])
		}
	}
}
