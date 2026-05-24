package providers

import (
	"testing"
	"time"
)

func TestAWSBedrockSourceHash_Deterministic(t *testing.T) {
	a := awsBedrockSourceHash("2026-05-20T00:00:00Z", "123", "Amazon Bedrock", "usage", "", "")
	b := awsBedrockSourceHash("2026-05-20T00:00:00Z", "123", "Amazon Bedrock", "usage", "", "")
	if a != b {
		t.Fatalf("same inputs changed hash: %s vs %s", a, b)
	}
	cases := []struct {
		name, date, account, service, usage, operation, region string
	}{
		{"date", "2026-05-21T00:00:00Z", "123", "Amazon Bedrock", "usage", "", ""},
		{"account", "2026-05-20T00:00:00Z", "456", "Amazon Bedrock", "usage", "", ""},
		{"service", "2026-05-20T00:00:00Z", "123", "Other", "usage", "", ""},
		{"usage", "2026-05-20T00:00:00Z", "123", "Amazon Bedrock", "other", "", ""},
		{"operation", "2026-05-20T00:00:00Z", "123", "Amazon Bedrock", "usage", "InvokeModel", ""},
		{"region", "2026-05-20T00:00:00Z", "123", "Amazon Bedrock", "usage", "", "us-west-2"},
	}
	for _, c := range cases {
		if got := awsBedrockSourceHash(c.date, c.account, c.service, c.usage, c.operation, c.region); got == a {
			t.Errorf("%s should change hash", c.name)
		}
	}
}

func TestAWSBedrock_ParseRegions(t *testing.T) {
	got := parseAWSRegions("us-east-1, us-west-2,US-EAST-1,,eu-central-1")
	want := []string{"us-east-1", "us-west-2", "eu-central-1"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestAWSBedrock_RegistryAndAlias(t *testing.T) {
	reg, ok := Get("aws-bedrock")
	if !ok {
		t.Fatal("aws-bedrock not registered")
	}
	alias, ok := Get("bedrock")
	if !ok || alias.Name != reg.Name {
		t.Fatalf("bedrock alias did not resolve: %+v ok=%v", alias, ok)
	}
	if _, ok := Get("aws"); ok {
		t.Fatal("generic aws alias must not be registered")
	}
}

func TestAWSBedrock_ClampAndWindow(t *testing.T) {
	p := NewAWSBedrockProvider(AWSBedrockConfig{})
	p.Now = func() time.Time { return time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC) }
	if clampAWSBedrockDays(0) != 1 || clampAWSBedrockDays(999) != awsBedrockMaxDays {
		t.Fatal("days clamp failed")
	}
	start, end := p.costWindow(1)
	if start.Format("2006-01-02") != "2026-05-24" || end.Format("2006-01-02") != "2026-05-25" {
		t.Fatalf("bad one-day window: %s to %s", start, end)
	}
}
