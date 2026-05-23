package providers

import "os"

func init() {
	Register(Registration{
		Name:         githubCopilotProviderName,
		Aliases:      []string{"copilot"},
		EnvVar:       "GITHUB_PAT",
		ExtraEnvVars: []string{"GITHUB_ORG"},
		MissingEnvHelp: "GITHUB_PAT and GITHUB_ORG must both be set.\n" +
			"Create a Personal Access Token with organization billing /\n" +
			"premium-request usage read permission (fine-grained: Administration:\n" +
			"Read at organization scope; classic PAT requirements vary), then:\n" +
			"  export GITHUB_PAT=ghp_...\n" +
			"  export GITHUB_ORG=your-org",
		New: func(adminKey string) Provider {
			// GITHUB_ORG is verified non-empty by fetchSelectedProviders before
			// calling this factory.
			org := os.Getenv("GITHUB_ORG")
			return NewGitHubCopilotProvider(adminKey, org)
		},
	})
}
