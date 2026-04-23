package executor

func GetInt() int        { return 42 }
func UseString(s string) {}

func MutateBad(x int) {
	UseString(x)
}
