package tui

import "strings"

// syncsBody renders sync freshness: the global last-sync age plus the most
// recent recorded activity per provider derived from cost_records. There is no
// sync-history table (by design), so this reflects activity, not a sync log.
func syncsBody(d Dashboard, s Styles, _ int) string {
	const labelW = 12
	head := labeled(s, "Last sync", labelW, syncAge(d.LastSync, d.GeneratedAt))

	if len(d.Syncs) == 0 {
		return head + "\n\n" + s.Faint.Render("No per-provider activity in the window.")
	}
	cols := []string{"Provider", "Latest activity"}
	rows := make([][]string, len(d.Syncs))
	for i, a := range d.Syncs {
		latest := "n/a"
		if !a.LatestActive.IsZero() {
			latest = age(d.GeneratedAt.Sub(a.LatestActive)) + " ago"
		}
		rows[i] = []string{a.Provider, latest}
	}
	return strings.Join([]string{head, "", table(s, cols, rows)}, "\n")
}
