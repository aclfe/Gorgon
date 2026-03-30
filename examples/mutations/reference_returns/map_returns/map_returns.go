package map_returns

func GetMap() map[string]int {
	return map[string]int{"a": 1, "b": 2}
}

func GetStringMap() map[string]string {
	return map[string]string{"key": "value"}
}

func GetEmptyMap() map[string]int {
	return map[string]int{}
}

func GetNilMap() map[string]int {
	return nil
}
