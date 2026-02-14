package condition_negation

import "testing"

func TestWeird(t *testing.T) {
	t.Parallel()
	if got := IsPositive(12); !got {
		t.Errorf("IsPositive(12) = %t, want true", got)
	}
}

func TestWeird2(t *testing.T) {
	t.Parallel()
	if got := IsNegative(-12); !got {
		t.Errorf("IsNegative(-12) = %t, want true", got)
	}
}
