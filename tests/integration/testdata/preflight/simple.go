package preflight

var X = 1
var Y = 2
var Z = 3

func GetValue() int {
	return X
}

func GetValues() (int, int, int) {
	return X, Y, Z
}
