package channel_returns

func GetChannel() chan int {
	return make(chan int)
}

func GetStringChannel() chan string {
	return make(chan string)
}

func GetNilChannel() chan int {
	return nil
}
