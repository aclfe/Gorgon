package swap_case_bodies

import "testing"

func TestGetDayName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		day  int
		want string
	}{
		{1, "Monday"},
		{2, "Tuesday"},
		{3, "Wednesday"},
		{4, "Thursday"},
		{5, "Friday"},
		{6, "Saturday"},
		{7, "Sunday"},
		{0, "Invalid"},
		{100, "Invalid"},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			if got := GetDayName(tt.day); got != tt.want {
				t.Errorf("GetDayName(%d) = %q, want %q", tt.day, got, tt.want)
			}
		})
	}
}

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
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			if got := GetGrade(tt.score); got != tt.want {
				t.Errorf("GetGrade(%d) = %q, want %q", tt.score, got, tt.want)
			}
		})
	}
}
