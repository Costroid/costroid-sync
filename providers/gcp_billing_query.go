package providers

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	gcpBQAPIVersion = "v2"
	gcpBQMaxPages   = 50
	gcpBQTimeoutMS  = 30000
	gcpBQMaxResults = 5000
)

// gcpTableIDRegex matches `project.dataset.table` where:
//   - project: GCP project ID — lowercase letters, digits, hyphens, 6-30 chars.
//   - dataset: starts with letter or underscore, then alnum/underscore.
//   - table:   same rule as dataset.
//
// Notes:
//   - All-numeric or 4-character project IDs are uncommon in GCP and
//     rejected here to keep the regex tight; users with such projects can
//     file an issue. The regex's purpose is defense against accidental
//     SQL injection, not letter-perfect spec compliance.
//   - This regex deliberately disallows backticks, semicolons, whitespace,
//     comment markers, and any character outside the explicit class.
//   - Dataset/table length capped at 255 chars (Go RE2 caps repeat
//     counts at 1000); BigQuery dataset names are limited to 1024 chars
//     in the spec, but 255 covers every realistic export name.
var gcpTableIDRegex = regexp.MustCompile(`^[a-z][a-z0-9-]{4,28}[a-z0-9]\.[A-Za-z_][A-Za-z0-9_]{0,254}\.[A-Za-z_][A-Za-z0-9_]{0,254}$`)

// validateGCPTableID rejects any string that doesn't match the strict
// project.dataset.table format. Defense against SQL injection via the
// table identifier (which is the only user-controlled value spliced into
// SQL — every other value goes through @parameters).
func validateGCPTableID(tableID string) error {
	if strings.TrimSpace(tableID) == "" {
		return fmt.Errorf("gcp-billing: %s not set", envGCPBillingTable)
	}
	// Belt-and-suspenders: reject obvious injection metacharacters BEFORE
	// any trimming, so a trailing newline can't sneak past the regex via
	// TrimSpace. If the regex were ever broadened by a future edit, these
	// explicit checks still hold.
	for _, ch := range []string{"`", ";", "--", "/*", "*/", "\\", " ", "\t", "\n", "\r"} {
		if strings.Contains(tableID, ch) {
			return fmt.Errorf("gcp-billing: %s contains invalid characters", envGCPBillingTable)
		}
	}
	if !gcpTableIDRegex.MatchString(tableID) {
		return fmt.Errorf("gcp-billing: %s must be in the form project.dataset.table", envGCPBillingTable)
	}
	return nil
}

// buildGCPQuerySQL constructs the parameterized SELECT for the billing
// table. The table ID is the ONLY interpolated value — every other input
// goes through @parameters. All selected columns are explicit and safe;
// labels, system_labels, tags, credits, adjustment_info, etc. are never
// referenced.
func buildGCPQuerySQL(tableID string, withProjectFilter bool) string {
	var b strings.Builder
	b.WriteString("SELECT ")
	b.WriteString("CAST(usage_start_time AS STRING) AS usage_start_time, ")
	b.WriteString("CAST(usage_end_time AS STRING) AS usage_end_time, ")
	b.WriteString("service.id AS service_id, ")
	b.WriteString("service.description AS service_description, ")
	b.WriteString("sku.id AS sku_id, ")
	b.WriteString("sku.description AS sku_description, ")
	b.WriteString("project.id AS project_id, ")
	b.WriteString("project.name AS project_name, ")
	b.WriteString("location.location AS location_location, ")
	b.WriteString("CAST(cost AS STRING) AS cost, ")
	b.WriteString("currency, ")
	b.WriteString("CAST(usage.amount AS STRING) AS usage_amount, ")
	b.WriteString("usage.unit AS usage_unit, ")
	b.WriteString("invoice.month AS invoice_month, ")
	b.WriteString("cost_type ")
	b.WriteString("FROM `")
	b.WriteString(tableID)
	b.WriteString("` ")
	b.WriteString("WHERE currency = @currency ")
	b.WriteString("AND DATE(usage_start_time) >= DATE_SUB(CURRENT_DATE(), INTERVAL @days DAY) ")
	if withProjectFilter {
		b.WriteString("AND project.id = @project_filter ")
	}
	b.WriteString("ORDER BY usage_start_time")
	return b.String()
}

// buildGCPQueryBody returns the request body for POST
// /bigquery/v2/projects/{project}/queries.
func buildGCPQueryBody(sql, currency string, days int, projectFilter string) map[string]any {
	params := []map[string]any{
		{
			"name":           "currency",
			"parameterType":  map[string]string{"type": "STRING"},
			"parameterValue": map[string]string{"value": currency},
		},
		{
			"name":           "days",
			"parameterType":  map[string]string{"type": "INT64"},
			"parameterValue": map[string]string{"value": strconv.Itoa(days)},
		},
	}
	if projectFilter != "" {
		params = append(params, map[string]any{
			"name":           "project_filter",
			"parameterType":  map[string]string{"type": "STRING"},
			"parameterValue": map[string]string{"value": projectFilter},
		})
	}
	return map[string]any{
		"query":           sql,
		"useLegacySql":    false,
		"parameterMode":   "NAMED",
		"queryParameters": params,
		"timeoutMs":       gcpBQTimeoutMS,
		"maxResults":      gcpBQMaxResults,
	}
}

// ---------- BigQuery response envelope ----------

type bqField struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type bqRowValue struct {
	V json.RawMessage `json:"v"`
}

type bqRow struct {
	F []bqRowValue `json:"f"`
}

type bqJobReference struct {
	ProjectID string `json:"projectId"`
	JobID     string `json:"jobId"`
	Location  string `json:"location"`
}

type bqQueryResponse struct {
	JobReference bqJobReference `json:"jobReference"`
	Schema       struct {
		Fields []bqField `json:"fields"`
	} `json:"schema"`
	Rows        []bqRow `json:"rows"`
	PageToken   string  `json:"pageToken"`
	JobComplete bool    `json:"jobComplete"`
	TotalRows   string  `json:"totalRows"`
}

// HTTP plumbing (fetchBillingRows / doGCPQuery / doGCPGetResults /
// doGCPHTTP) and row decoding (decodeGCPBillingRows /
// buildGCPColumnIndex / decodeGCPBillingRow / bqValueString /
// normalizeBQTimestamp) live in gcp_billing_query_http.go to keep this
// file under the 300-line limit.
