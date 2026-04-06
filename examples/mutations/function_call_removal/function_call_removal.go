package function_call_removal

import (
	"fmt"
	"os"
	"sync"
)

func LogStart() {
	fmt.Println("starting")
}

func ExitOnError() {
	os.Exit(1)
}

func WaitGroupDone(wg *sync.WaitGroup) {
	wg.Done()
}

func CloseChannel(ch chan int) {
	close(ch)
}

func MultiCall() {
	fmt.Println("before")
	fmt.Println("after")
}
