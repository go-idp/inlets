package protocol

import (
	"sync"
	"testing"
	"time"
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

func TestStreamManagerCleanupEvictsStall(t *testing.T) {
	sm := &StreamManager{
		streams:          make(map[string]*Stream),
		defaultChunkSize: 1024,
		maxStreamAge:     time.Hour,
		stallTimeout:     50 * time.Millisecond,
		stopCleanup:      make(chan struct{}),
	}
	s := NewStream("stale")
	s.State = StreamStateActive
	s.lastActivity = time.Now().Add(-time.Hour)
	s.createdAt = time.Now().Add(-time.Hour)
	sm.streams["stale"] = s

	var errSeen error
	s.OnError = func(e error) { errSeen = e }

	sm.cleanup()
	if sm.GetStream("stale") != nil {
		t.Fatal("stale stream should be removed")
	}
	if errSeen == nil {
		t.Fatal("expected OnError on eviction")
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
