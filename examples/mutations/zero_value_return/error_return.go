package zero_value_return

import "fmt"

func GetError() error {
	return fmt.Errorf("something failed")
}

func GetNil() error {
	return nil
}
