package interface_returns

func GetInterface() interface{} {
	return "hello"
}

func GetIntInterface() interface{} {
	return 42
}

func GetNilInterface() interface{} {
	return nil // equivalent mutant
}
