package providers

import (
	"errors"
	"fmt"
	"strings"
)

// azureHTTPError is the only error type Azure HTTP failures emit. It
// holds the status code and the API path — never the response body, the
// bearer token, or the client secret. See the SECURITY notes in
// azure_openai_auth.go and azure_openai_query.go.
type azureHTTPError struct {
	StatusCode int
	Endpoint   string // path only; never includes query string with secrets
}

func (e *azureHTTPError) Error() string {
	return fmt.Sprintf("azure-openai %s: HTTP %d", e.Endpoint, e.StatusCode)
}

// wrapAzureTokenPermissionHint adds a friendly help message for the four
// "likely permission/availability" statuses from the OAuth token endpoint
// while preserving the underlying azureHTTPError via %w. 500-class stays
// raw.
func wrapAzureTokenPermissionHint(err error) error {
	var he *azureHTTPError
	if errors.As(err, &he) {
		switch he.StatusCode {
		case 400, 401, 403, 404:
			return fmt.Errorf("%w: Microsoft Entra token request failed. "+
				"Check AZURE_TENANT_ID, AZURE_CLIENT_ID, AZURE_CLIENT_SECRET, "+
				"and that the service principal exists and is enabled", err)
		}
	}
	return err
}

// wrapAzureManagementPermissionHint does the same for the management
// (Cost Management / Monitor) endpoint statuses.
func wrapAzureManagementPermissionHint(err error) error {
	var he *azureHTTPError
	if errors.As(err, &he) {
		switch he.StatusCode {
		case 401, 403, 404:
			return fmt.Errorf("%w: Azure management API request failed. "+
				"Check that the service principal has the 'Cost Management Reader' role "+
				"on the subscription or scope (and 'Monitoring Reader' on each Azure OpenAI "+
				"resource if AZURE_OPENAI_RESOURCE_IDS is set)", err)
		}
	}
	return err
}

// normalizeAzureScope strips a leading slash from s and trims trailing
// slashes. So "subscriptions/X", "/subscriptions/X", and
// "/subscriptions/X/" all normalize to "subscriptions/X".
func normalizeAzureScope(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "/")
	s = strings.TrimRight(s, "/")
	return s
}
