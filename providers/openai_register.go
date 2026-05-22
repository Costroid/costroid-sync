package providers

func init() {
	Register(Registration{
		Name:   "openai",
		EnvVar: "OPENAI_ADMIN_KEY",
		MissingEnvHelp: "OPENAI_ADMIN_KEY is not set.\n" +
			"Create an admin key at https://platform.openai.com/settings/organization/admin-keys, then:\n" +
			"  export OPENAI_ADMIN_KEY=sk-admin-...",
		New: func(adminKey string) Provider {
			return NewOpenAIProvider(adminKey)
		},
	})
}
