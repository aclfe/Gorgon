package zero_value_return

func GetNumber() int {
	return 42
}

func GetString() string {
	return "hello"
}

func GetSlice() []int {
	return []int{1, 2, 3}
}

func GetMap() map[string]int {
	return map[string]int{"a": 1}
}
