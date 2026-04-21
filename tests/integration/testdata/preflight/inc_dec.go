package preflight

func Increment(x int) int {
	x++
	return x
}

func Decrement(y int) int {
	y--
	return y
}

func UseBoth(a, b int) int {
	a++
	b--
	return a + b
}
