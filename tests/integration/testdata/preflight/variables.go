package preflight

var globalVar = 10
var anotherVar = 20

func GetGlobal() int {
	return globalVar
}

func UseBoth() int {
	return globalVar + anotherVar
}
