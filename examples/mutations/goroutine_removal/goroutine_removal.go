package goroutine_removal

import (
	"context"
	"sync"
	"time"
)

func StartWorker(done chan bool) {
	go func() {
		time.Sleep(50 * time.Millisecond)
		done <- true
	}()
}

func LaunchMultiple(ctx context.Context, results chan string) {
	go sender(ctx, results, "hello")
	go sender(ctx, results, "world")
}

func sender(ctx context.Context, results chan string, msg string) {
	time.Sleep(50 * time.Millisecond)
	select {
	case results <- msg:
	case <-ctx.Done():
	}
}

func AnonymousGoroutine(done chan bool) {
	go func() {
		time.Sleep(50 * time.Millisecond)
		done <- true
	}()
}

func WaitGroupGoroutine(wg *sync.WaitGroup, result chan int) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(50 * time.Millisecond)
		result <- 100
	}()
}

func ConcurrentSum(a, b int, result chan int) {
	var va, vb int
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		va = compute(a)
	}()
	go func() {
		defer wg.Done()
		vb = compute(b)
	}()
	wg.Wait()
	result <- va + vb
}

func compute(n int) int {
	time.Sleep(50 * time.Millisecond)
	return n * 2
}
