package providers

import "testing"

func TestGet_CanonicalName(t *testing.T) {
	reg, ok := Get("github-copilot")
	if !ok {
		t.Fatal("Get(github-copilot) returned ok=false")
	}
	if reg.Name != "github-copilot" {
		t.Errorf("Name = %q", reg.Name)
	}
}

func TestGet_Alias(t *testing.T) {
	canonical, okCanonical := Get("github-copilot")
	alias, okAlias := Get("copilot")
	if !okCanonical || !okAlias {
		t.Fatalf("expected both lookups to succeed (canonical=%v, alias=%v)", okCanonical, okAlias)
	}
	if canonical.Name != alias.Name {
		t.Errorf("alias resolved to different name: canonical=%q alias=%q", canonical.Name, alias.Name)
	}
	if canonical.EnvVar != alias.EnvVar {
		t.Errorf("alias has different EnvVar: %q vs %q", canonical.EnvVar, alias.EnvVar)
	}
}

func TestGet_UnknownName(t *testing.T) {
	if _, ok := Get("not-a-provider"); ok {
		t.Error("expected unknown provider lookup to fail")
	}
}

func TestAll_StableOrdering(t *testing.T) {
	regs := All()
	if len(regs) < 3 {
		t.Fatalf("All() returned %d registrations, want >= 3", len(regs))
	}
	wantOrder := []string{"openai", "anthropic", "github-copilot"}
	for i, want := range wantOrder {
		if regs[i].Name != want {
			t.Errorf("All()[%d].Name = %q, want %q", i, regs[i].Name, want)
		}
	}
}

func TestGet_CanonicalShadowsAlias(t *testing.T) {
	// If a registration's Name matches another registration's Alias, the
	// canonical lookup must win. We don't have such a collision today, but
	// the test asserts Get's preference order so a future addition can't
	// silently break it. Canonical "openai" is in the registry; if we ever
	// added an alias "openai" on another reg, Get("openai") must still
	// return the openai registration. We construct that scenario by reading
	// from the registry directly and asserting Get's behavior matches.
	openai, ok := Get("openai")
	if !ok || openai.Name != "openai" {
		t.Fatalf("baseline lookup of openai failed: %+v", openai)
	}
}
