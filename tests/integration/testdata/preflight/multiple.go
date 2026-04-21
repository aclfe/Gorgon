package preflight

var (
	A = 1
	B = 2
	C = 3
	D = 4
	E = 5
)

func Values() (int, int, int, int, int) {
	return A, B, C, D, E
}

func Sum() int {
	return A + B + C + D + E
}
