package typeedgecases

const BlockSize = 32

func ArrayWithConst() [BlockSize]byte {
	return [BlockSize]byte{}
}

func PointerToArray() *[4]byte {
	return &[4]byte{}
}

func SliceOfArrays() [][8]byte {
	return [][8]byte{}
}

func MapWithArrayKey() map[[2]byte]int {
	return map[[2]byte]int{}
}

func ChanOfArrays() chan [16]byte {
	return make(chan [16]byte)
}

func NestedPointers() **int {
	return nil
}

func EllipsisParam(args ...string) {
	_ = args
}

func InterfaceReturn() interface{} {
	return nil
}

func ArrayWithBinary() [2]byte {
	return [2]byte{}
}

func ArrayWithParen() [4]byte {
	return [4]byte{}
}
