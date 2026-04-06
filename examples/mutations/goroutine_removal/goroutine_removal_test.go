package goroutine_removal

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestStartWorker(t *testing.T) {
	t.Parallel()
	done := make(chan bool, 1)
	start := time.Now()
	StartWorker(done)
	elapsed := time.Since(start)
	if elapsed > 10*time.Millisecond {
		t.Fatal("StartWorker should return immediately")
	}
	select {
	case v := <-done:
		if !v {
			t.Error("expected true")
		}
	case <-time.After(time.Second):
		t.Fatal("worker did not complete")
	}
}

func TestLaunchMultiple(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	results := make(chan string, 2)
	start := time.Now()
	LaunchMultiple(ctx, results)
	elapsed := time.Since(start)
	if elapsed > 10*time.Millisecond {
		t.Fatal("LaunchMultiple should return immediately")
	}
	received := make(map[string]bool)
	for i := 0; i < 2; i++ {
		select {
		case msg := <-results:
			received[msg] = true
		case <-time.After(time.Second):
			t.Fatal("sender did not complete")
		}
	}
	if !received["hello"] || !received["world"] {
		t.Errorf("expected hello and world, got %v", received)
	}
}

func TestAnonymousGoroutine(t *testing.T) {
	t.Parallel()
	done := make(chan bool, 1)
	start := time.Now()
	AnonymousGoroutine(done)
	elapsed := time.Since(start)
	if elapsed > 10*time.Millisecond {
		t.Fatal("AnonymousGoroutine should return immediately")
	}
	select {
	case v := <-done:
		if !v {
			t.Error("expected true")
		}
	case <-time.After(time.Second):
		t.Fatal("anonymous goroutine did not complete")
	}
}

func TestWaitGroupGoroutine(t *testing.T) {
	t.Parallel()
	var wg sync.WaitGroup
	result := make(chan int, 1)
	WaitGroupGoroutine(&wg, result)
	wg.Wait()
	select {
	case v := <-result:
		if v != 100 {
			t.Errorf("got %d, want 100", v)
		}
	case <-time.After(time.Second):
		t.Fatal("goroutine did not send result")
	}
}

func TestConcurrentSum(t *testing.T) {
	t.Parallel()
	result := make(chan int, 1)
	start := time.Now()
	ConcurrentSum(3, 4, result)
	elapsed := time.Since(start)
	if elapsed > 80*time.Millisecond {
		t.Fatalf("ConcurrentSum took %v, expected ~50ms (concurrent)", elapsed)
	}
	select {
	case v := <-result:
		if v != 14 {
			t.Errorf("got %d, want 14", v)
		}
	case <-time.After(time.Second):
		t.Fatal("ConcurrentSum did not complete")
	}
}
