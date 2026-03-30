package channel_returns

import "testing"

func TestGetStringChannel(t *testing.T) {
	t.Parallel()
	if got := GetStringChannel(); got == nil {
		t.Errorf("GetStringChannel() = nil, want non-nil channel")
	}
}

