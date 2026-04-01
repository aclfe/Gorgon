package sign_toggle

import "testing"

func TestNegate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input int
		want  int
	}{
		{5, -5},
		{-3, 3},
		{0, 0},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			if got := Negate(tt.input); got != tt.want {
				t.Errorf("Negate(%d) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestDoubleNegate(t *testing.T) {
	t.Parallel()
	if got := DoubleNegate(5); got != 5 {
		t.Errorf("DoubleNegate(5) = %d, want 5", got)
	}
}

func TestPositive(t *testing.T) {
	t.Parallel()
	if got := Positive(5); got != 5 {
		t.Errorf("Positive(5) = %d, want 5", got)
	}
}
