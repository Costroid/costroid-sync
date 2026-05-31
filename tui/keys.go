package tui

import "github.com/charmbracelet/bubbles/key"

// keyMap is the authoritative keyboard map (t1-feasibility §5.3). It implements
// bubbles/help.KeyMap so the help component can render it. The TUI is
// keyboard-first; mouse support is intentionally not enabled.
type keyMap struct {
	Up     key.Binding
	Down   key.Binding
	Left   key.Binding
	Right  key.Binding
	Top    key.Binding
	Bottom key.Binding
	Jump   key.Binding
	Help   key.Binding
	Quit   key.Binding
}

// newKeyMap builds the keyboard map. The help labels for the cursor keys use
// arrow glyphs in UTF-8 mode and degrade to the plain letter keys under --plain
// so the help footer stays ASCII-safe (the key bindings themselves are unchanged).
func newKeyMap(ascii bool) keyMap {
	up, down, left, right := "↑/k", "↓/j", "←/h", "→/l"
	if ascii {
		up, down, left, right = "k", "j", "h", "l"
	}
	return keyMap{
		Up:     key.NewBinding(key.WithKeys("up", "k"), key.WithHelp(up, "scroll up")),
		Down:   key.NewBinding(key.WithKeys("down", "j"), key.WithHelp(down, "scroll down")),
		Left:   key.NewBinding(key.WithKeys("left", "h", "shift+tab"), key.WithHelp(left, "prev panel")),
		Right:  key.NewBinding(key.WithKeys("right", "l", "tab"), key.WithHelp(right, "next panel")),
		Top:    key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", "top")),
		Bottom: key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "bottom")),
		Jump:   key.NewBinding(key.WithKeys("1", "2", "3", "4", "5", "6", "7", "8", "9", "0"), key.WithHelp("1-9,0", "jump to panel")),
		Help:   key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:   key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

// ShortHelp implements help.KeyMap (the inline footer hint).
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Right, k.Down, k.Jump, k.Help, k.Quit}
}

// FullHelp implements help.KeyMap (the expanded overlay shown on "?").
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Top, k.Bottom},
		{k.Left, k.Right, k.Jump},
		{k.Help, k.Quit},
	}
}
