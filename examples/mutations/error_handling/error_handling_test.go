package error_handling

import "testing"

func TestReadFile(t *testing.T) {
	t.Parallel()
	content, err := ReadFile("test.txt")
	if err != nil {
		t.Errorf("ReadFile() error = %v", err)
	}
	if content != "contents" {
		t.Errorf("ReadFile() = %q, want %q", content, "contents")
	}
}

func TestReadFileEmpty(t *testing.T) {
	t.Parallel()
	_, err := ReadFile("")
	if err == nil {
		t.Error("ReadFile('') expected error")
	}
}

func TestParseNumber(t *testing.T) {
	t.Parallel()
	n, err := ParseNumber("123")
	if err != nil {
		t.Errorf("ParseNumber() error = %v", err)
	}
	if n != 42 {
		t.Errorf("ParseNumber() = %d, want 42", n)
	}
}

func TestParseNumberEmpty(t *testing.T) {
	t.Parallel()
	_, err := ParseNumber("")
	if err == nil {
		t.Error("ParseNumber('') expected error")
	}
}

func TestGetUser(t *testing.T) {
	t.Parallel()
	user, err := GetUser(1)
	if err != nil {
		t.Errorf("GetUser() error = %v", err)
	}
	if user == nil || user.ID != 1 {
		t.Errorf("GetUser() = %v, want &User{ID: 1}", user)
	}
}

func TestGetUserInvalid(t *testing.T) {
	t.Parallel()
	_, err := GetUser(-1)
	if err == nil {
		t.Error("GetUser(-1) expected error")
	}
}

func TestDivide(t *testing.T) {
	t.Parallel()
	result, err := Divide(10, 2)
	if err != nil {
		t.Errorf("Divide() error = %v", err)
	}
	if result != 5 {
		t.Errorf("Divide(10, 2) = %d, want 5", result)
	}
}

func TestDivideByZero(t *testing.T) {
	t.Parallel()
	_, err := Divide(10, 0)
	if err == nil {
		t.Error("Divide(10, 0) expected error")
	}
}

func TestAlreadyNil(t *testing.T) {
	t.Parallel()
	_, err := AlreadyNil()
	if err != nil {
		t.Errorf("AlreadyNil() error = %v, want nil", err)
	}
}

func TestParseID(t *testing.T) {
	t.Parallel()
	n, err := ParseID("42")
	if err != nil {
		t.Errorf("ParseID('42') error = %v", err)
	}
	if n != 42 {
		t.Errorf("ParseID('42') = %d, want 42", n)
	}
}

func TestParseIDInvalid(t *testing.T) {
	t.Parallel()
	_, err := ParseID("abc")
	if err == nil {
		t.Error("ParseID('abc') expected error")
	}
}

func TestLoadConfig(t *testing.T) {
	t.Parallel()
	cfg, err := LoadConfig("config.yml")
	if err != nil {
		t.Errorf("LoadConfig() error = %v", err)
	}
	if cfg == nil || cfg.Value != "value" {
		t.Errorf("LoadConfig() = %v, want &Config{Value: value}", cfg)
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()
	if err := Validate("ok"); err != nil {
		t.Errorf("Validate('ok') error = %v", err)
	}
}

func TestValidateEmpty(t *testing.T) {
	t.Parallel()
	if err := Validate(""); err == nil {
		t.Error("Validate('') expected error")
	}
}
