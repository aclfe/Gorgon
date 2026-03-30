package channel_returns

func GetChannel() chan int {
	ch := make(chan int)
	_ = ch
	return ch
}

func GetStringChannel() chan string {
	ch := make(chan string)
	_ = ch
	return ch
}

func GetNilChannel() chan int {
	return nil
}
