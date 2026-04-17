package testdata

// Add returns the sum of two integers
func Add(a, b int) int {
	return a + b
}

// Subtract returns the difference of two integers
func Subtract(a, b int) int {
	return a - b
}

// SkipThis should be skipped by the sub-config
func SkipThis() string {
	return "should be skipped"
}
