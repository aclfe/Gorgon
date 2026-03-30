package pointer_returns

func GetPointer() *int {
	x := 42
	_ = x
	return &x
}

func GetStringPointer() *string {
	s := "hello"
	_ = s
	return &s
}

func GetNilPointer() *int {
	return nil
}
