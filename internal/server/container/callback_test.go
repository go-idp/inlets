package container

import "testing"

func TestCallbackContainerTakeConsumesOnce(t *testing.T) {
	c := NewCallbackContainer()

	called := 0
	c.Set("tcp-1", "req-1", func(data string) {
		if data != "ok" {
			t.Fatalf("unexpected callback data: %s", data)
		}
		called++
	})

	cb := c.Take("tcp-1", "req-1")
	if cb == nil {
		t.Fatal("expected callback to be returned on first Take")
	}
	cb("ok")

	if called != 1 {
		t.Fatalf("expected callback to be called once, got %d", called)
	}

	if c.Get("tcp-1", "req-1") != nil {
		t.Fatal("expected callback to be removed after Take")
	}

	if c.Take("tcp-1", "req-1") != nil {
		t.Fatal("expected second Take to return nil")
	}
}

