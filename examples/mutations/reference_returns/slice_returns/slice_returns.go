package slice_returns

func GetSlice() []int {
	return []int{1, 2, 3}
}

func GetStringSlice() []string {
	return []string{"a", "b", "c"}
}

func GetEmptySlice() []int {
	return []int{}
}

func GetNilSlice() []int {
	return nil
}
