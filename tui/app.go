package tui

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// Layout thresholds. The full metric-row dashboard targets >= 80x24; between a
// usable minimum and that it collapses to a single summary row; below the tiny
// minimum it shows a one-line "too small" message (t1-feasibility §7).
const (
	headerHeight  = 2 // brand line + tab line
	fullMinWidth  = 80
	fullMinHeight = 24
	tinyMinWidth  = 20
	tinyMinHeight = 3
)

// Options configures a TUI run. Tier (the resolved color capability) and ASCII are
// resolved by the caller from --plain, NO_COLOR, COLORTERM/TERM, and the locale
// (see ResolveTier). NoAnimation is accepted and honored as a documented no-op in
// T1.2 (the dashboard has no animation by rule).
type Options struct {
	Tier        ColorTier
	ASCII       bool
	NoAnimation bool
	Now         time.Time
}

// panelDef is one navigable dashboard panel. tab is the short label shown in
// the tab bar (kept short so eight tabs fit an 80-column row); title is the full
// heading shown above the panel body. body renders metadata only.
type panelDef struct {
	num   string
	tab   string
	title string
	body  func(Dashboard, Styles, int) string
}

// panelRegistry lists the navigable panels in tab order. tab labels are kept
// short (abbreviated) so all ten fit one 80-column tab row; title is the full
// heading shown above the body. The tenth panel's jump key is "0" (1-9 then 0).
func panelRegistry() []panelDef {
	return []panelDef{
		{"1", "ovw", "Overview", overviewBody},
		{"2", "prov", "Providers", providersBody},
		{"3", "models", "Models", modelsBody},
		{"4", "budget", "Budget", budgetBody},
		{"5", "fcast", "Forecast", forecastBody},
		{"6", "anom", "Anomalies", anomaliesBody},
		{"7", "hist", "History", historyBody},
		{"8", "trend", "Trend", trendBody},
		{"9", "syncs", "Recent Syncs", syncsBody},
		{"0", "export", "Export hints", exportHintsBody},
	}
}

type model struct {
	data   Dashboard
	styles Styles
	keys   keyMap
	help   help.Model
	vp     viewport.Model
	panels []panelDef
	active int
	width  int
	height int
	ready  bool
}

func newModel(d Dashboard, opts Options) model {
	return model{
		data:   d,
		styles: newStyles(surfaceCold, opts.Tier, opts.ASCII),
		keys:   newKeyMap(opts.ASCII),
		help:   newHelp(opts.ASCII),
		vp:     viewport.New(0, 0),
		panels: panelRegistry(),
	}
}

// newHelp builds the help-footer component, forcing ASCII separators under
// --plain so the footer carries no non-ASCII glyph (bubbles/help defaults to a
// "•" bullet and a "…" ellipsis). UTF-8 mode keeps the library defaults.
func newHelp(ascii bool) help.Model {
	h := help.New()
	if ascii {
		h.ShortSeparator = " | "
		h.Ellipsis = "..."
	}
	return h
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.Width = msg.Width
		m.ready = true
		m.relayout()
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Help):
		m.help.ShowAll = !m.help.ShowAll
		m.relayout()
		return m, nil
	case key.Matches(msg, m.keys.Right):
		m.setActive((m.active + 1) % len(m.panels))
		return m, nil
	case key.Matches(msg, m.keys.Left):
		m.setActive((m.active - 1 + len(m.panels)) % len(m.panels))
		return m, nil
	case key.Matches(msg, m.keys.Top):
		m.vp.GotoTop()
		return m, nil
	case key.Matches(msg, m.keys.Bottom):
		m.vp.GotoBottom()
		return m, nil
	case key.Matches(msg, m.keys.Jump):
		if i := jumpIndex(msg.String(), len(m.panels)); i >= 0 {
			m.setActive(i)
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg) // j/k/arrows/page scroll the active panel
	return m, cmd
}

// setActive switches the active panel, refreshes the viewport content, and
// scrolls back to the top.
func (m *model) setActive(i int) {
	m.active = i
	m.vp.SetContent(m.activeBody())
	m.vp.GotoTop()
}

// relayout recomputes the viewport dimensions from the current terminal size
// and the (possibly expanded) help footer height, then refreshes content.
func (m *model) relayout() {
	if !m.ready {
		return
	}
	bodyH := m.height - headerHeight - lineCount(m.footerView()) - 2 // 2 blank separators
	if bodyH < 1 {
		bodyH = 1
	}
	m.vp.Width = m.width
	m.vp.Height = bodyH
	m.vp.SetContent(m.activeBody())
}

// activeBody renders the active panel's heading + metadata-only body.
func (m model) activeBody() string {
	p := m.panels[m.active]
	return m.styles.Title.Render(p.title) + "\n\n" + p.body(m.data, m.styles, m.width)
}

// jumpIndex maps a single-digit key to a panel index: '1'..'9' select panels
// 0..8 and '0' selects the tenth panel (index 9). It returns -1 for any other
// key or an index beyond the current panel count.
func jumpIndex(s string, n int) int {
	if len(s) != 1 || s[0] < '0' || s[0] > '9' {
		return -1
	}
	i := int(s[0] - '1')
	if s[0] == '0' {
		i = 9
	}
	if i >= 0 && i < n {
		return i
	}
	return -1
}

// Run loads the dashboard from read-only local SQLite and launches the
// alternate-screen program. Bubble Tea restores the terminal (leaves the alt
// screen, restores cooked mode, shows the cursor) on every exit path — normal
// quit, Ctrl-C/SIGINT, SIGTERM, and panic. It performs no network, provider, or
// credential access.
func Run(ctx context.Context, opts Options) error {
	if opts.Now.IsZero() {
		opts.Now = time.Now().UTC()
	}
	data := LoadDashboard(ctx, opts.Now)
	p := tea.NewProgram(newModel(data, opts), tea.WithAltScreen(), tea.WithContext(ctx))
	_, err := p.Run()
	return err
}

// InteractiveAllowed reports whether a fullscreen TUI may be launched. It
// requires both stdin and stdout to be TTYs and a usable TERM; a pipe or
// TERM=dumb must refuse rather than paint an alternate screen into a pipe
// (t1-feasibility §7). This pure decision keeps the non-TTY guard testable.
func InteractiveAllowed(stdoutTTY, stdinTTY bool, term string) bool {
	if !stdoutTTY || !stdinTTY {
		return false
	}
	return strings.TrimSpace(strings.ToLower(term)) != "dumb"
}
