package condition_negation

func IsPositive(n int) bool {
	return n > 0
}

func IsNegative(n int) bool {
	return n < 0
}

func IsEqual(a, b int) bool {
	return a == b
}

func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
