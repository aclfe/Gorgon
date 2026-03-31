package empty_body

import "testing"

func TestProcess(t *testing.T) {
	t.Parallel()
	data := []byte{1, 2, 3}
	Process(data)
}

func TestInitialize(t *testing.T) {
	t.Parallel()
	initialize()
}

func TestCleanup(t *testing.T) {
	t.Parallel()
	cleanup()
}

func TestNoOp(t *testing.T) {
	t.Parallel()
	noOp()
}
