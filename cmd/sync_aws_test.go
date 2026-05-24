package cmd

import (
	"context"
	"strings"
	"testing"
)

func TestSyncAWSBedrockMissingEnvSafeError(t *testing.T) {
	clearProviderEnv(t)
	regs, err := selectedRegistrations("aws-bedrock")
	if err != nil {
		t.Fatalf("selectedRegistrations: %v", err)
	}
	_, _, err = fetchSelectedProviders(context.Background(), regs, 1, false)
	if err == nil {
		t.Fatal("expected missing env error")
	}
	msg := err.Error()
	for _, want := range []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("missing %s in error: %s", want, msg)
		}
	}
	if strings.Contains(msg, "test-secret") || strings.Contains(msg, "AKIA") {
		t.Fatalf("error leaked credential-looking value: %s", msg)
	}
}

func TestSyncAllNoCredentialsMentionsAWSBedrock(t *testing.T) {
	clearProviderEnv(t)
	regs, err := selectedRegistrations("all")
	if err != nil {
		t.Fatalf("selectedRegistrations: %v", err)
	}
	_, _, err = fetchSelectedProviders(context.Background(), regs, 1, true)
	if err == nil {
		t.Fatal("expected no credentials error")
	}
	if !strings.Contains(err.Error(), "AWS_ACCESS_KEY_ID + AWS_SECRET_ACCESS_KEY") {
		t.Fatalf("error missing AWS env hint: %s", err)
	}
}

func clearProviderEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"OPENAI_ADMIN_KEY", "ANTHROPIC_ADMIN_KEY",
		"GITHUB_PAT", "GITHUB_ORG",
		"GEMINI_BILLING_EXPORT",
		"AZURE_TENANT_ID", "AZURE_CLIENT_ID", "AZURE_CLIENT_SECRET", "AZURE_SUBSCRIPTION_ID",
		"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY",
	} {
		t.Setenv(name, "")
	}
}
