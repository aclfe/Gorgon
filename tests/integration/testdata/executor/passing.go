package executor

var GlobalVar = 10

func GetValue() int { return GlobalVar }

func Add(a, b int) int { return a + b }

func Use(x int) int { return x + 1 }
