package defer_removal

import (
	"bytes"
	"os"
	"sync"
)

type Resource struct {
	closed   bool
	mu       sync.Mutex
	closeBuf *bytes.Buffer
}

func (r *Resource) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	if r.closeBuf != nil {
		r.closeBuf.WriteString("closed")
	}
	return nil
}

func (r *Resource) IsClosed() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closed
}

var resource Resource

func OpenResource() *Resource {
	return &resource
}

func ProcessWithCleanup() {
	r := OpenResource()
	defer r.Close()
	_ = r.IsClosed()
}

func MultipleDefers(buf *bytes.Buffer) {
	r := OpenResource()
	r.closeBuf = buf
	defer r.Close()
	defer buf.WriteString("after")
}

func InFunction() error {
	file, _ := os.Open("test.txt")
	defer file.Close()
	return nil
}
