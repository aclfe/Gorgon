package function_call_removal

import (
	"bytes"
	"io"
	"os"
	"sync"
	"testing"
	"time"
)

func captureOutput(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	f()
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestLogStart(t *testing.T) {
	t.Parallel()
	output := captureOutput(LogStart)
	if output != "starting\n" {
		t.Errorf("LogStart() output = %q, want %q", output, "starting\n")
	}
}

func TestWaitGroupDone(t *testing.T) {
	t.Parallel()
	var wg sync.WaitGroup
	wg.Add(1)
	WaitGroupDone(&wg)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("WaitGroup not done")
	}
}

func TestCloseChannel(t *testing.T) {
	t.Parallel()
	ch := make(chan int)
	go func() {
		CloseChannel(ch)
	}()
	_, ok := <-ch
	if ok {
		t.Error("expected channel to be closed")
	}
}

func TestMultiCall(t *testing.T) {
	t.Parallel()
	output := captureOutput(MultiCall)
	if output != "before\nafter\n" {
		t.Errorf("MultiCall() output = %q, want %q", output, "before\nafter\n")
	}
}
