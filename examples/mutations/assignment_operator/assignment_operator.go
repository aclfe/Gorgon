package assignment_operator

func AddToCounter(counter *int, value int) {
	*counter += value
}

func Double(value int) int {
	value *= 2
	return value
}

func Triple(value int) int {
	value = value * 3
	return value
}

func Halve(value int) int {
	if value == 0 {
		return 0
	}
	value /= 2
	return value
}
