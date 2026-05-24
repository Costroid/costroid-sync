package providers

import "os"

func init() {
	Register(Registration{
		Name:         awsBedrockProviderName,
		Aliases:      []string{"bedrock"},
		EnvVar:       envAWSAccessKeyID,
		ExtraEnvVars: []string{envAWSSecretAccessKey},
		MissingEnvHelp: "AWS Bedrock requires AWS credentials:\n" +
			"  export AWS_ACCESS_KEY_ID=...\n" +
			"  export AWS_SECRET_ACCESS_KEY=...\n" +
			"Optional:\n" +
			"  export AWS_SESSION_TOKEN=...\n" +
			"  export AWS_COST_EXPLORER_REGION=us-east-1\n" +
			"  export AWS_BEDROCK_REGIONS=us-east-1,us-west-2\n" +
			"  export AWS_ACCOUNT_ID=123456789012\n" +
			"Grant ce:GetCostAndUsage for Cost Explorer and, if AWS_BEDROCK_REGIONS is set,\n" +
			"cloudwatch:ListMetrics plus cloudwatch:GetMetricData for token metric enrichment.",
		New: func(accessKeyID string) Provider {
			return NewAWSBedrockProvider(AWSBedrockConfig{
				AccessKeyID:        accessKeyID,
				SecretAccessKey:    os.Getenv(envAWSSecretAccessKey),
				SessionToken:       os.Getenv(envAWSSessionToken),
				Region:             os.Getenv(envAWSRegion),
				CostExplorerRegion: os.Getenv(envAWSCostExplorerRegion),
				AccountID:          os.Getenv(envAWSAccountID),
				MetricRegions:      parseAWSRegions(os.Getenv(envAWSBedrockRegions)),
			})
		},
	})
}
