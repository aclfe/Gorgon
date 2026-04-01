package if_condition_true

func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func Sign(x int) int {
	if x > 0 {
		return 1
	} else {
		return 0
	}
}

func SimpleCondition(n int) {
	if n > 10 {
		println("big")
	}
}