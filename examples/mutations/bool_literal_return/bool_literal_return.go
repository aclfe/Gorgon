package bool_literal_return

func IsPositive(n int) bool {
	return n > 0
}

func HasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func IsEqual(a, b int) bool {
	return a == b
}

func IsEmpty(s string) bool {
	return len(s) == 0
}
