package loop_break_removal

func ClassicBreak(result *int) {
	for i := 0; i < 10; i++ {
		if i == 5 {
			break
		}
		*result += i
	}
}

func RangeBreak(result *int, items []int) {
	for _, v := range items {
		if v < 0 {
			break
		}
		*result += v
	}
}

func NestedBreak(result *int) {
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			if i == j {
				break
			}
			*result += 1
		}
	}
}

func MultipleBreaks(found *bool) {
	for i := 0; i < 10; i++ {
		if i == 3 {
			break
		}
		if i == 5 {
			break
		}
	}
	*found = true
}
