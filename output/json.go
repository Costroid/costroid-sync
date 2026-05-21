package output

import (
	"encoding/json"
	"io"

	"github.com/costroid/costroid-sync/providers"
)

// WriteJSON serialises records as a JSON array. Real formatting and
// stable field ordering land in C6.
func WriteJSON(w io.Writer, records []providers.NormalizedCostRecord) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(records)
}
