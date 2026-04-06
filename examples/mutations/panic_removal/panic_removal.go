package panic_removal

func RequirePositive(n int) {
	if n <= 0 {
		//gorgon:ignore panic_removal:9
		panic("must be positive")
	}
}

func RequireNotZero(n int) {
	if n == 0 {
		//gorgon:ignore panic_removal
		panic("must not be zero")
	}
}

func GetFromMap(m map[string]int, key string) int {
	val, ok := m[key]
	if !ok {
		panic("key not found")
	}
	return val
}

func GetItem(items []string, i int) string {
	if i >= len(items) {
		panic("out of range")
	}
	return items[i]
}

func MustNonEmpty(s string) {
	if s == "" {
		panic("empty string")
	}
}
