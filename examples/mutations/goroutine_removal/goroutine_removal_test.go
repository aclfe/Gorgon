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
	
	// Use a channel to detect if function returns before work completes
	returned := make(chan struct{})
	go func() {
		StartWorker(done)
		close(returned)
	}()
	
	select {
	case <-returned:
	case <-done:
		t.Fatal("StartWorker should return before goroutine completes")
	}
	
	select {
	case v := <-done:
		if !v {
			t.Error("expected true")
		}
	}
}

func TestLaunchMultiple(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	results := make(chan string, 2)
	
	returned := make(chan struct{})
	go func() {
		LaunchMultiple(ctx, results)
		close(returned)
	}()
	
	select {
	case <-returned:
	case <-results:
		t.Fatal("LaunchMultiple should return before goroutines complete")
	}
	
	received := make(map[string]bool)
	for i := 0; i < 2; i++ {
		select {
		case msg := <-results:
			received[msg] = true
		default:
		}
	}

	for i := 0; i < 10 && len(received) < 2; i++ {
		select {
		case msg := <-results:
			received[msg] = true
		}
	}
	if !received["hello"] || !received["world"] {
		t.Errorf("expected hello and world, got %v", received)
	}
}

func TestAnonymousGoroutine(t *testing.T) {
	t.Parallel()
	done := make(chan bool, 1)
	
	returned := make(chan struct{})
	go func() {
		AnonymousGoroutine(done)
		close(returned)
	}()
	
	select {
	case <-returned:
	case <-done:
		t.Fatal("AnonymousGoroutine should return before goroutine completes")
	}
	
	select {
	case v := <-done:
		if !v {
			t.Error("expected true")
		}
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
	}
}

func TestConcurrentSum(t *testing.T) {
	t.Parallel()
	result := make(chan int, 1)
		
	startChan := make(chan struct{})
	barrier1 := make(chan struct{})
	barrier2 := make(chan struct{})
	overlap := make(chan bool, 1)
	
	go func() {
		<-startChan
		barrier1 <- struct{}{}
		time.Sleep(50 * time.Millisecond)
		<-barrier2
	}()
	
	go func() {
		<-startChan
		barrier2 <- struct{}{}
		time.Sleep(50 * time.Millisecond)
	}()
	
	close(startChan)
	
	<-barrier1
	<-barrier2
	
	select {
	case <-overlap:
	default:
	}
	
	ConcurrentSum(3, 4, result)
	select {
	case v := <-result:
		if v != 14 {
			t.Errorf("got %d, want 14", v)
		}
	}
}
