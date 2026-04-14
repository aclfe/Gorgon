package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestMainHandlesFlagErrors(t *testing.T) {
	if os.Getenv("GORGON_SUB") == "1" {
		
		os.Args = []string{"gorgon", "-invalidflag"}
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestMainHandlesFlagErrors$")
	cmd.Env = append(os.Environ(), "GORGON_SUB=1")
	out, err := cmd.CombinedOutput()

	
	if err == nil {
		t.Fatal("expected main() to exit with error for invalid flag, but it succeeded")
	}

	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got %T", err)
	}

	if exitErr.ExitCode() != 1 {
		t.Fatalf("expected exit code 1, got %d", exitErr.ExitCode())
	}

	
	output := string(out)
	if !strings.Contains(output, "flag provided but not defined") {
		t.Fatalf("expected flag parsing error message, got:\n%s", output)
	}
}



func TestMainHandlesValidationErrors(t *testing.T) {
	if os.Getenv("GORGON_SUB") == "1" {
		
		os.Args = []string{"gorgon", "-threshold=invalid"}
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestMainHandlesValidationErrors$")
	cmd.Env = append(os.Environ(), "GORGON_SUB=1")
	out, err := cmd.CombinedOutput()

	
	if err == nil {
		t.Fatal("expected main() to exit with error for invalid threshold, but it succeeded")
	}

	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got %T", err)
	}

	if exitErr.ExitCode() != 1 {
		t.Fatalf("expected exit code 1, got %d", exitErr.ExitCode())
	}

	output := string(out)
	if output == "" {
		t.Fatal("expected error output to be non-empty")
	}
}
