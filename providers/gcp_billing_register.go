package providers

import "os"

func init() {
	Register(Registration{
		Name:    gcpBillingProviderName,
		Aliases: []string{"gcp"},
		// EnvVar holds the env-var name whose value is the file path to a
		// service-account JSON key. Mechanism mirrors other providers; the
		// path is not itself a secret, but the file it points to is.
		EnvVar: envGCPServiceAccountJSON,
		ExtraEnvVars: []string{
			envGCPBillingProject,
			envGCPBillingTable,
		},
		MissingEnvHelp: "GCP Billing requires three env vars:\n" +
			"  export GCP_SERVICE_ACCOUNT_JSON=/path/to/service-account.json\n" +
			"  export GCP_BILLING_PROJECT=your-query-project\n" +
			"  export GCP_BILLING_TABLE=your-project.billing_export_data.gcp_billing_export_v1_XXXXXX\n" +
			"Optional:\n" +
			"  export GCP_BILLING_PROJECT_FILTER=your-gcp-project-id\n" +
			"  export GCP_BILLING_SERVICE_FILTER=Vertex AI,Gemini,Cloud Run\n" +
			"  export GCP_BILLING_CURRENCY=USD\n" +
			"The service account needs 'BigQuery Data Viewer' on the export\n" +
			"dataset and 'BigQuery Job User' on GCP_BILLING_PROJECT. Cloud\n" +
			"Billing export to BigQuery must be enabled on the billing\n" +
			"account first.",
		New: func(serviceAccountPath string) Provider {
			return NewGCPBillingProvider(GCPBillingConfig{
				ServiceAccountJSONPath: serviceAccountPath,
				BillingProject:         os.Getenv(envGCPBillingProject),
				BillingTable:           os.Getenv(envGCPBillingTable),
				ProjectFilter:          os.Getenv(envGCPBillingProjectFlt),
				ServiceFilters:         parseServiceFilters(os.Getenv(envGCPBillingServiceFlt)),
				Currency:               os.Getenv(envGCPBillingCurrency),
			})
		},
	})
}
