package defer_removal

import (
	"bytes"
	"io"
	"testing"
)

func TestResourceClose(t *testing.T) {
	t.Parallel()
	r := &Resource{}
	err := r.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}
	if !r.IsClosed() {
		t.Error("expected resource to be closed")
	}
}

func TestProcessWithCleanup(t *testing.T) {
	t.Parallel()
	ProcessWithCleanup()
}

func TestMultipleDefers(t *testing.T) {
	t.Parallel()
	buf := &bytes.Buffer{}
	MultipleDefers(buf)
	if buf.Len() == 0 {
		t.Error("expected buf to have content")
	}
}

func TestInFunction(t *testing.T) {
	t.Parallel()
	if err := InFunction(); err != nil {
		t.Errorf("InFunction() error = %v", err)
	}
}

var _ io.Closer = (*Resource)(nil)
