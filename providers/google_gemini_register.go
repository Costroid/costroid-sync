package providers

import "os"

func init() {
	Register(Registration{
		Name:    googleGeminiProviderName,
		Aliases: []string{"gemini"},
		// EnvVar holds the env-var *name* whose value is the file path
		// to a Google Cloud Billing CSV export. The "EnvVar" field is
		// named for secret credentials in other providers; for Gemini
		// it's just a file path. Mechanism is the same.
		EnvVar: envGeminiBillingExport,
		MissingEnvHelp: "GEMINI_BILLING_EXPORT not set.\n" +
			"Google Gemini uses Cloud Billing export files rather than a live API.\n" +
			"Export Cloud Billing data from BigQuery as CSV, then:\n" +
			"  export GEMINI_BILLING_EXPORT=/path/to/google-billing-export.csv",
		New: func(exportPath string) Provider {
			p := NewGoogleGeminiProvider(exportPath)
			p.ProjectFilter = os.Getenv(envGeminiBillingProject)
			if filter := os.Getenv(envGeminiBillingServiceFilter); filter != "" {
				p.ServiceFilters = parseServiceFilters(filter)
			}
			return p
		},
	})
}
