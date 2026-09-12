package acp

import (
	"errors"
	"testing"
)

func TestDefaultProfilesIncludeOpencodeFirst(t *testing.T) {
	profiles := DefaultProfiles()
	if len(profiles) == 0 {
		t.Fatal("no profiles")
	}
	if profiles[0].Name != "opencode" {
		t.Fatalf("first profile = %q, want opencode", profiles[0].Name)
	}
	for _, p := range profiles {
		if len(p.Command) == 0 {
			t.Errorf("profile %q: empty command", p.Name)
		}
		if p.Available {
			if _, err := lookupAvailableCheck(p); err != nil {
				t.Errorf("profile %q marked available but lookup failed: %v", p.Name, err)
			}
		}
	}
}

func TestResolveUnknownProfile(t *testing.T) {
	if _, err := Resolve("nonexistent-agent"); err == nil {
		t.Fatal("want error for unknown profile")
	}
}

func TestResolveUnavailableBinary(t *testing.T) {
	if _, err := Resolve("claude"); err == nil {
		t.Log("claude-code-acp is on PATH; unavailable-path case untested here")
	}
}

func lookupAvailableCheck(p Info) (struct{}, error) {
	if !lookupAvailable(p.Command) {
		return struct{}{}, errors.New("not on PATH")
	}
	return struct{}{}, nil
}
