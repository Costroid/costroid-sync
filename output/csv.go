package output

import (
	"errors"
	"io"

	"github.com/costroid/costroid-sync/providers"
)

// ErrNotImplemented is returned by output stubs not yet wired up.
var ErrNotImplemented = errors.New("output not implemented yet")

// WriteCSV writes records as CSV. Real implementation (including
// FOCUS format) lands in C6.
func WriteCSV(w io.Writer, records []providers.NormalizedCostRecord) error {
	return ErrNotImplemented
}
