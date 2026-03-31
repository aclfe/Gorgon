package variable_replacement

func Calculate(a, b int) int {
	result := a + b
	temp := a - b
	return result * temp
}

func ProcessValues(x, y int) int {
	diff := x - y
	sum := x + y
	if diff > 0 {
		return sum
	}
	return diff
}

func FindMax(first, second int) int {
	max := first
	if second > max {
		max = second
	}
	return max
}
