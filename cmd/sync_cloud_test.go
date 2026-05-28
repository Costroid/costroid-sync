package cmd

import "testing"

func TestCloudPushConfigDefaultsAPIURL(t *testing.T) {
	t.Setenv(envCostroidAPIURL, "")
	t.Setenv(envCostroidOrgID, "org-123")
	t.Setenv(envCostroidAgentKey, "csk_test")

	cfg, missing := cloudPushConfigFromEnv()
	if len(missing) != 0 {
		t.Fatalf("missing = %v, want none", missing)
	}
	if cfg.BaseURL != defaultCostroidAPIURL {
		t.Fatalf("base URL = %q, want %q", cfg.BaseURL, defaultCostroidAPIURL)
	}
	if cfg.OrgID != "org-123" || cfg.AgentKey != "csk_test" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestCloudPushConfigRejectsOldOrgKeyName(t *testing.T) {
	t.Setenv(envCostroidAPIURL, "https://example.test")
	t.Setenv(envCostroidOrgID, "org-123")
	t.Setenv(envCostroidAgentKey, "")
	t.Setenv("COSTROID_ORG_KEY", "old-schema-key")

	_, missing := cloudPushConfigFromEnv()
	if len(missing) != 1 || missing[0] != envCostroidAgentKey {
		t.Fatalf("missing = %v, want [%s]", missing, envCostroidAgentKey)
	}
}
