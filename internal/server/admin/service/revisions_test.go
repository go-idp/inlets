package service

import (
	"strings"
	"testing"
)

func TestUnifiedDiff_EmptyBoth(t *testing.T) {
	d := UnifiedDiff("", "")
	if d != "" {
		t.Errorf("expected empty diff, got %q", d)
	}
}

func TestUnifiedDiff_AddLine(t *testing.T) {
	d := UnifiedDiff("a\n", "a\nb\n")
	if !strings.Contains(d, "+b") {
		t.Errorf("expected +b in diff, got %q", d)
	}
	if !strings.Contains(d, " a") {
		t.Errorf("expected ' a' (kept) in diff, got %q", d)
	}
}

func TestUnifiedDiff_RemoveLine(t *testing.T) {
	d := UnifiedDiff("a\nb\n", "a\n")
	if !strings.Contains(d, "-b") {
		t.Errorf("expected -b in diff, got %q", d)
	}
}

func TestUnifiedDiff_NoChange(t *testing.T) {
	d := UnifiedDiff("a\nb\n", "a\nb\n")
	if strings.Contains(d, "+") || strings.Contains(d, "-") {
		t.Errorf("expected no +/- in unchanged diff, got %q", d)
	}
}

func TestUnifiedDiff_Modify(t *testing.T) {
	d := UnifiedDiff("a\nold\nb\n", "a\nnew\nb\n")
	if !strings.Contains(d, "-old") {
		t.Errorf("expected -old, got %q", d)
	}
	if !strings.Contains(d, "+new") {
		t.Errorf("expected +new, got %q", d)
	}
}

func TestUnifiedDiff_SecretNotLeakedWhenCallerPassesMasked(t *testing.T) {
	// The diff library doesn't know about secrets. The caller is
	// responsible for passing already-masked content. This test asserts
	// the contract: if we pass masked content, the diff doesn't grow
	// any leakage of the unmasked secret.
	const masked = "domain: example.com\nclients:\n  - clientId: a\n    clientSecret: '***'\n"
	const newMasked = "domain: example.com\nclients:\n  - clientId: a\n    clientSecret: '***'\n    tunnels:\n      - type: http\n        upstream: 127.0.0.1:80\n"
	d := UnifiedDiff(masked, newMasked)
	if strings.Contains(d, "shhh-actual-secret") {
		t.Errorf("diff should not contain raw secret, got %q", d)
	}
}
