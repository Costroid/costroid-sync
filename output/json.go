package output

import (
	"encoding/json"
	"io"

	"github.com/costroid/costroid/providers"
)

// WriteJSON serialises records as a metadata-only JSON array.
func WriteJSON(w io.Writer, records []providers.NormalizedCostRecord) error {
	if records == nil {
		records = []providers.NormalizedCostRecord{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(records)
}
