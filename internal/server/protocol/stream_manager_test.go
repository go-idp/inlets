package protocol

import (
	"sync"
	"testing"
)

func TestStreamManagerAddChunkWithoutStreamDoesNotDeadlock(t *testing.T) {
	sm := NewStreamManager(1024)
	defer sm.Destroy()

	done := make(chan struct{})
	go func() {
		sm.AddChunk("missing", 0, []byte("a"), true)
		close(done)
	}()
	<-done

	if sm.GetStream("missing") != nil {
		t.Fatal("stream must not be auto-created")
	}
}

func TestStreamManagerEnsureStreamThenAddChunkReassembles(t *testing.T) {
	sm := NewStreamManager(1024)
	defer sm.Destroy()

	var got []byte
	var wg sync.WaitGroup
	wg.Add(1)

	sm.EnsureStream("s1", func(data []byte) {
		got = append([]byte(nil), data...)
		wg.Done()
	}, nil)

	sm.AddChunk("s1", 0, []byte("hel"), false)
	sm.AddChunk("s1", 1, []byte("lo"), true)

	wg.Wait()
	if string(got) != "hello" {
		t.Fatalf("reassembled %q want hello", got)
	}
}

func TestStreamManagerEnsureStreamFirstCallbackWins(t *testing.T) {
	sm := NewStreamManager(1024)
	defer sm.Destroy()

	var first, second bool
	sm.EnsureStream("s2", func([]byte) { first = true }, nil)
	sm.EnsureStream("s2", func([]byte) { second = true }, nil)

	sm.AddChunk("s2", 0, []byte("x"), true)
	if !first {
		t.Fatal("expected first onComplete to be kept")
	}
	if second {
		t.Fatal("second onComplete must not replace first")
	}
}
