package for_condition_true

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

func InfiniteLoop() int {
	x := 0
	for true {
		x++
		if x > 100 {
			break
		}
	}
	return x
}