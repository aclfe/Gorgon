package switch_remove_default

import "testing"


func TestGetGrade(t *testing.T) {
	t.Parallel()
	tests := []struct {
		score int
		want  string
	}{
		{95, "A"},
		{85, "B"},
		{75, "C"},
		{65, "D"},
		{50, "F"},
		{0, "F"},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			if got := GetGrade(tt.score); got != tt.want {
				t.Errorf("GetGrade(%d) = %q, want %q", tt.score, got, tt.want)
			}
		})
	}
}

func TestProcessValue(t *testing.T) {
	t.Parallel()
	tests := []struct {
		val  interface{}
		want string
	}{
		{42, "integer"},
		{"hello", "string"},
		{3.14, "float"},
		{[]int{1, 2}, "unknown"},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			if got := ProcessValue(tt.val); got != tt.want {
				t.Errorf("ProcessValue(%v) = %q, want %q", tt.val, got, tt.want)
			}
		})
	}
}
