package for_condition_false

func SumUntilLimit(counter int, limit int) int {
	sum := 0
	for counter < limit {
		sum += counter
		counter++
	}
	return sum
}

func LoopWithCondition(i int) int {
	result := 0
	for i > 0 {
		result += i
		i--
	}
	return result
}