package error_handling

import (
	"errors"
	"fmt"
	"strconv"
)

func ReadFile(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("empty filename")
	}
	return "contents", nil
}

func ParseNumber(s string) (int, error) {
	if s == "" {
		return 0, errors.New("empty string")
	}
	return 42, nil
}

func GetUser(id int) (*User, error) {
	if id <= 0 {
		return nil, &AppError{Msg: "invalid id"}
	}
	return &User{ID: id}, nil
}

func Divide(a, b int) (int, error) {
	if b == 0 {
		return 0, ErrDivByZero
	}
	return a / b, nil
}

func AlreadyNil() (string, error) {
	return "", nil
}

func SingleReturn() error {
	return fmt.Errorf("single error return")
}

func ParseID(raw string) (int, error) {
	id, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func LoadConfig(path string) (*Config, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, err
	}
	cfg, err := parseConfig(data)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	return cfg, nil
}

func Validate(v string) error {
	if v == "" {
		return errors.New("empty")
	}
	return nil
}

type User struct {
	ID int
}

type AppError struct {
	Msg string
}

func (e *AppError) Error() string {
	return e.Msg
}

type Config struct {
	Value string
}

var ErrDivByZero = errors.New("division by zero")

func readFile(path string) ([]byte, error) {
	return []byte("key=value"), nil
}

func parseConfig(data []byte) (*Config, error) {
	return &Config{Value: "value"}, nil
}
