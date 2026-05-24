package providers

import (
	"os"
	"strings"
)

func init() {
	Register(Registration{
		Name:   azureProviderName,
		EnvVar: envAzureClientSecret,
		ExtraEnvVars: []string{
			envAzureTenantID,
			envAzureClientID,
			envAzureSubscriptionID,
		},
		MissingEnvHelp: "Azure OpenAI requires four env vars:\n" +
			"  export AZURE_TENANT_ID=...\n" +
			"  export AZURE_CLIENT_ID=...\n" +
			"  export AZURE_CLIENT_SECRET=...\n" +
			"  export AZURE_SUBSCRIPTION_ID=...\n" +
			"Optional:\n" +
			"  export AZURE_COST_SCOPE=subscriptions/<id>\n" +
			"  export AZURE_OPENAI_RESOURCE_IDS=/subscriptions/.../Microsoft.CognitiveServices/accounts/...\n" +
			"Create a service principal with the 'Cost Management Reader' role on the\n" +
			"subscription (and 'Monitoring Reader' on each Azure OpenAI resource if you\n" +
			"want token metric enrichment).",
		New: func(clientSecret string) Provider {
			subscriptionID := os.Getenv(envAzureSubscriptionID)
			scope := os.Getenv(envAzureCostScope)
			if scope == "" {
				scope = "subscriptions/" + subscriptionID
			}
			var resourceIDs []string
			if raw := os.Getenv(envAzureOpenAIResources); raw != "" {
				for _, id := range strings.Split(raw, ",") {
					if s := strings.TrimSpace(id); s != "" {
						resourceIDs = append(resourceIDs, s)
					}
				}
			}
			return NewAzureOpenAIProvider(AzureOpenAIConfig{
				TenantID:       os.Getenv(envAzureTenantID),
				ClientID:       os.Getenv(envAzureClientID),
				ClientSecret:   clientSecret,
				SubscriptionID: subscriptionID,
				Scope:          scope,
				ResourceIDs:    resourceIDs,
			})
		},
	})
}
