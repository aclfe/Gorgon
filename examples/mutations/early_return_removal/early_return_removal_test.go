package early_return_removal

import "testing"

func TestProcess(t *testing.T) {
	t.Parallel()
	data := []byte{1, 2, 3}
	if err := Process(data); err != nil {
		t.Errorf("Process() error = %v", err)
	}
}

func TestProcessNil(t *testing.T) {
	t.Parallel()
	if err := Process(nil); err == nil {
		t.Error("Process(nil) expected error")
	}
}

func TestValidateInput(t *testing.T) {
	t.Parallel()
	if err := ValidateInput(50); err != nil {
		t.Errorf("ValidateInput(50) error = %v", err)
	}
}

func TestValidateInputNegative(t *testing.T) {
	t.Parallel()
	if err := ValidateInput(-1); err == nil {
		t.Error("ValidateInput(-1) expected error")
	}
}

func TestValidateInputTooLarge(t *testing.T) {
	t.Parallel()
	if err := ValidateInput(101); err == nil {
		t.Error("ValidateInput(101) expected error")
	}
}

func TestCheck(t *testing.T) {
	t.Parallel()
	if err := Check("valid"); err != nil {
		t.Errorf("Check() error = %v", err)
	}
}

func TestCheckEmpty(t *testing.T) {
	t.Parallel()
	if err := Check(""); err == nil {
		t.Error("Check('') expected error")
	}
}
