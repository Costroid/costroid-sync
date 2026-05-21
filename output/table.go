package output

import "github.com/charmbracelet/lipgloss"

// HeaderStyle is the lipgloss style used for table column headers.
// Consumed by history, trend, and export rendering in C5/C6.
var HeaderStyle = lipgloss.NewStyle().Bold(true)
