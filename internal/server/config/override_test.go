package config

import (
	"strings"
	"sync"
	"testing"
)

func TestOverride_SetAndApplyTopLevel(t *testing.T) {
	o := NewOverride()
	if err := o.Set("domain", "override.example.com"); err != nil {
		t.Fatalf("set: %v", err)
	}
	base := &FileConfig{Domain: "orig.example.com"}
	out := o.Apply(base)
	if out.Domain != "override.example.com" {
		t.Errorf("expected override applied, got %q", out.Domain)
	}
	if base.Domain != "orig.example.com" {
		t.Errorf("base was mutated: %q", base.Domain)
	}
}

func TestOverride_SetUnknownFieldFails(t *testing.T) {
	o := NewOverride()
	if err := o.Set("notARealField", "x"); err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestOverride_NestedClientSecret(t *testing.T) {
	o := NewOverride()
	if err := o.Set("clients[0].clientSecret", "new-secret"); err != nil {
		t.Fatalf("set: %v", err)
	}
	base := &FileConfig{
		Domain: "x.com",
		Clients: []ClientConfig{
			{ClientID: "a", ClientSecret: "old"},
		},
	}
	out := o.Apply(base)
	if out.Clients[0].ClientSecret != "new-secret" {
		t.Errorf("expected new-secret, got %q", out.Clients[0].ClientSecret)
	}
	if base.Clients[0].ClientSecret != "old" {
		t.Errorf("base was mutated")
	}
}

func TestOverride_DurationField(t *testing.T) {
	o := NewOverride()
	if err := o.Set("publicHTTPNoAuth.timeout", "30m"); err != nil {
		t.Fatalf("set: %v", err)
	}
	base := &FileConfig{Domain: "x.com"}
	out := o.Apply(base)
	if out.PublicHTTPNoAuth == nil || out.PublicHTTPNoAuth.Timeout != "30m" {
		t.Errorf("expected 30m timeout, got %+v", out.PublicHTTPNoAuth)
	}
}

func TestOverride_NilBase(t *testing.T) {
	o := NewOverride()
	out := o.Apply(nil)
	if out != nil {
		t.Errorf("expected nil for nil base")
	}
}

func TestOverride_EmptyPatches(t *testing.T) {
	o := NewOverride()
	base := &FileConfig{Domain: "x.com"}
	out := o.Apply(base)
	if out.Domain != "x.com" {
		t.Errorf("expected unchanged, got %q", out.Domain)
	}
}

func TestOverride_Delete(t *testing.T) {
	o := NewOverride()
	if err := o.Set("domain", "x.com"); err != nil {
		t.Fatalf("set: %v", err)
	}
	o.Delete("domain")
	if o.Size() != 0 {
		t.Errorf("expected empty, got %d", o.Size())
	}
}

func TestOverride_ClearAll(t *testing.T) {
	o := NewOverride()
	_ = o.Set("domain", "x.com")
	_ = o.Set("port", 8080)
	o.ClearAll()
	if o.Size() != 0 {
		t.Errorf("expected empty, got %d", o.Size())
	}
}

func TestOverride_List(t *testing.T) {
	o := NewOverride()
	_ = o.Set("domain", "x.com")
	_ = o.Set("port", 8080)
	entries := o.List()
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}
}

func TestOverride_ConcurrentSetApply(t *testing.T) {
	o := NewOverride()
	base := &FileConfig{Domain: "x.com"}
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = o.Set("port", i)
				}
			}
		}(i)
	}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = o.Apply(base)
				}
			}
		}()
	}
	close(stop)
	wg.Wait()
}

func TestOverride_BoolField(t *testing.T) {
	o := NewOverride()
	if err := o.Set("secure", false); err != nil {
		t.Fatalf("set: %v", err)
	}
	yes := true
	base := &FileConfig{Domain: "x.com", Secure: &yes}
	out := o.Apply(base)
	if out.Secure == nil {
		t.Fatal("expected non-nil")
	}
	if *out.Secure != false {
		t.Errorf("expected false, got %v", *out.Secure)
	}
}

func TestOverride_EmptyPathFails(t *testing.T) {
	o := NewOverride()
	if err := o.Set("", "x"); err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestOverride_PathResolvesRejectsUnknownSubpath(t *testing.T) {
	o := NewOverride()
	err := o.Set("clients[0].nonexistent", "x")
	if err == nil {
		t.Fatal("expected error for unknown subfield")
	}
	if !strings.Contains(err.Error(), "does not resolve") {
		t.Errorf("unexpected error: %v", err)
	}
}
