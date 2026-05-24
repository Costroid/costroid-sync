package providers

import (
	"strings"
	"testing"
)

func TestGCPBillingRegister_Canonical(t *testing.T) {
	reg, ok := Get(gcpBillingProviderName)
	if !ok {
		t.Fatalf("Get(%q) returned ok=false", gcpBillingProviderName)
	}
	if reg.Name != gcpBillingProviderName {
		t.Errorf("Name = %q", reg.Name)
	}
	if reg.EnvVar != envGCPServiceAccountJSON {
		t.Errorf("EnvVar = %q, want %q", reg.EnvVar, envGCPServiceAccountJSON)
	}
}

func TestGCPBillingRegister_Alias(t *testing.T) {
	canonical, okCanonical := Get("gcp-billing")
	alias, okAlias := Get("gcp")
	if !okCanonical || !okAlias {
		t.Fatalf("expected both lookups to succeed (canonical=%v, alias=%v)",
			okCanonical, okAlias)
	}
	if canonical.Name != alias.Name {
		t.Errorf("alias resolved to different name: canonical=%q alias=%q",
			canonical.Name, alias.Name)
	}
}

func TestGCPBillingRegister_ExtraEnvVars(t *testing.T) {
	reg, ok := Get(gcpBillingProviderName)
	if !ok {
		t.Fatal("Get returned ok=false")
	}
	have := map[string]bool{}
	for _, v := range reg.ExtraEnvVars {
		have[v] = true
	}
	for _, want := range []string{envGCPBillingProject, envGCPBillingTable} {
		if !have[want] {
			t.Errorf("ExtraEnvVars missing %q; got %v", want, reg.ExtraEnvVars)
		}
	}
}

// TestGCPBillingRegister_MissingEnvHelpListsNamesOnly asserts the help
// text references env-var NAMES only — never sample values that the user
// might have already exported and could be confidential.
func TestGCPBillingRegister_MissingEnvHelpListsNamesOnly(t *testing.T) {
	reg, ok := Get(gcpBillingProviderName)
	if !ok {
		t.Fatal("Get returned ok=false")
	}
	help := reg.MissingEnvHelp
	for _, want := range []string{
		envGCPServiceAccountJSON, envGCPBillingProject, envGCPBillingTable,
		envGCPBillingProjectFlt, envGCPBillingServiceFlt, envGCPBillingCurrency,
	} {
		if !strings.Contains(help, want) {
			t.Errorf("help text missing %q: %s", want, help)
		}
	}
}

// TestProviderOrderIncludesGCPBilling makes sure All() returns the new
// provider in the canonical position so `sync --provider all` ordering
// is stable across runs.
func TestProviderOrderIncludesGCPBilling(t *testing.T) {
	regs := All()
	names := make([]string, 0, len(regs))
	for _, r := range regs {
		names = append(names, r.Name)
	}
	// Find google-gemini and assert gcp-billing sits immediately after it.
	gemIdx := -1
	for i, n := range names {
		if n == "google-gemini" {
			gemIdx = i
			break
		}
	}
	if gemIdx < 0 {
		t.Fatalf("google-gemini missing from provider order: %v", names)
	}
	if gemIdx+1 >= len(names) || names[gemIdx+1] != "gcp-billing" {
		t.Errorf("gcp-billing not immediately after google-gemini; order = %v", names)
	}
}
