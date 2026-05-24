package providers

import (
	"errors"
	"fmt"
)

// gcpBillingHTTPError is the only error type GCP Billing HTTP failures
// emit. It carries the status code and the API path — never the response
// body, the bearer token, the JWT assertion, the private key, or the
// service-account email. See SECURITY notes in gcp_billing_auth.go and
// gcp_billing_query.go.
type gcpBillingHTTPError struct {
	StatusCode int
	Endpoint   string // path only; never includes query string with tokens
}

func (e *gcpBillingHTTPError) Error() string {
	return fmt.Sprintf("gcp-billing %s: HTTP %d", e.Endpoint, e.StatusCode)
}

// wrapGCPBillingTokenPermissionHint adds a friendly help message for the
// four "likely permission/availability" statuses from the Google OAuth
// token endpoint while preserving the underlying gcpBillingHTTPError via
// %w. 500-class stays raw.
func wrapGCPBillingTokenPermissionHint(err error) error {
	var he *gcpBillingHTTPError
	if errors.As(err, &he) {
		switch he.StatusCode {
		case 400, 401, 403, 404:
			return fmt.Errorf("%w: Google OAuth token request failed. "+
				"Check GCP_SERVICE_ACCOUNT_JSON points to a valid service-account "+
				"key file and that the service account is enabled", err)
		}
	}
	return err
}

// wrapGCPBillingQueryPermissionHint adds a friendly help message for the
// four "likely permission/availability" statuses from the BigQuery REST
// API while preserving the underlying gcpBillingHTTPError via %w. The
// hint references env var names only — never values.
func wrapGCPBillingQueryPermissionHint(err error) error {
	var he *gcpBillingHTTPError
	if errors.As(err, &he) {
		switch he.StatusCode {
		case 400, 401, 403, 404:
			return fmt.Errorf("%w: BigQuery REST request failed. "+
				"Check GCP_BILLING_PROJECT and GCP_BILLING_TABLE are correct, "+
				"the table exists, and the service account has "+
				"'BigQuery Data Viewer' on the export dataset and "+
				"'BigQuery Job User' on the query project", err)
		}
	}
	return err
}
