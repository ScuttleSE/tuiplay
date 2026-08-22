package config

import "testing"

func TestResolveKeysDefaults(t *testing.T) {
	m, err := ResolveKeys(nil)
	if err != nil {
		t.Fatal(err)
	}
	if m[ActConsume] != "R" || m[ActRepeat] != "r" {
		t.Fatalf("defaults wrong: %v", m)
	}
}

func TestResolveKeysOverride(t *testing.T) {
	m, err := ResolveKeys(map[string]string{ActConsume: "C"})
	if err != nil {
		t.Fatal(err)
	}
	if m[ActConsume] != "C" {
		t.Fatalf("override not applied: %v", m[ActConsume])
	}
}

func TestResolveKeysConflict(t *testing.T) {
	// Bind repeat to the same key as consume (R).
	_, err := ResolveKeys(map[string]string{ActRepeat: "R"})
	if err == nil {
		t.Fatal("expected conflict error")
	}
}

func TestResolveKeysUnknownAction(t *testing.T) {
	_, err := ResolveKeys(map[string]string{"frobnicate": "x"})
	if err == nil {
		t.Fatal("expected unknown-action error")
	}
}

func TestResolveKeysEmptyKey(t *testing.T) {
	_, err := ResolveKeys(map[string]string{ActConsume: ""})
	if err == nil {
		t.Fatal("expected empty-key error")
	}
}
