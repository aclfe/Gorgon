package channel_returns

import "testing"

func TestGetChannel(t *testing.T) {
	t.Parallel()
	if got := GetChannel(); got == nil {
		t.Errorf("GetChannel() = nil, want non-nil channel")
	}
}

func TestGetStringChannel(t *testing.T) {
	t.Parallel()
	if got := GetStringChannel(); got == nil {
		t.Errorf("GetStringChannel() = nil, want non-nil channel")
	}
}

