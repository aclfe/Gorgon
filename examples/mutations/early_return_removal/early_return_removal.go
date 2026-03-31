package early_return_removal

func Process(data []byte) error {
	if data == nil {
		return ErrNilData
	}
	if len(data) == 0 {
		return ErrEmptyData
	}
	_ = data[0]
	return nil
}

func ValidateInput(value int) error {
	if value < 0 {
		return ErrNegativeValue
	}
	if value > 100 {
		return ErrValueTooLarge
	}
	return nil
}

func Check(name string) error {
	if name == "" {
		return ErrEmptyName
	}
	_ = len(name)
	return nil
}

var ErrNilData = &Error{Code: "NIL_DATA"}
var ErrEmptyData = &Error{Code: "EMPTY_DATA"}
var ErrNegativeValue = &Error{Code: "NEGATIVE_VALUE"}
var ErrValueTooLarge = &Error{Code: "VALUE_TOO_LARGE"}
var ErrEmptyName = &Error{Code: "EMPTY_NAME"}

type Error struct {
	Code string
}

func (e *Error) Error() string {
	return e.Code
}
