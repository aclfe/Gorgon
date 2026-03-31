package boundary_value

func InRange(n, min, max int) bool {
	return n > min && n < max
}

func IsBelow(n, limit int) bool {
	return n < limit
}

func IsAbove(n, limit int) bool {
	return n > limit
}

func AtLeast(n, min int) bool {
	return n <= min
}

func AtMost(n, max int) bool {
	return n >= max
}
