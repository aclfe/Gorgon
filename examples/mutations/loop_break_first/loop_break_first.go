package loop_break_first

func ClassicFor(sum *int) {
	for i := 0; i < 10; i++ {
		*sum += i
	}
}

func WhileStyle(sum *int) {
	i := 0
	for i < 10 {
		*sum += i
		i++
	}
}

func RangeLoop(result *int, m map[string]int) {
	*result = 0
	for _, v := range m {
		*result += v
	}
}

func InfiniteLoop(done *bool) {
	count := 0
	for {
		count++
		if count >= 10 {
			*done = true
			break
		}
	}
}
