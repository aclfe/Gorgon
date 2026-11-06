package testing

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

var hashBufPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 32*1024)
		return &buf
	},
}

func copyFileWithBuffer(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", src, err)
	}
	defer srcFile.Close()

	mode := os.FileMode(0o644)
	if info, err := srcFile.Stat(); err == nil {
		mode = info.Mode().Perm() | 0o200
	}
	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", dst, err)
	}
	defer dstFile.Close()

	bufPtr := hashBufPool.Get().(*[]byte)
	defer hashBufPool.Put(bufPtr)

	if _, err := io.CopyBuffer(dstFile, srcFile, *bufPtr); err != nil {
		return fmt.Errorf("failed to copy %s: %w", src, err)
	}
	return nil
}

func FindGoModDir(dir string) string {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(absDir, "go.mod")); err == nil {
			return absDir
		}
		parent := filepath.Dir(absDir)
		if parent == absDir {
			break
		}
		absDir = parent
	}
	return ""
}

func UniqueErrorLines(output string, skipPrefix string) []string {
	var errs []string
	seen := make(map[string]bool)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasSuffix(line, "too many errors") {
			continue
		}
		if skipPrefix != "" && strings.HasPrefix(line, skipPrefix) {
			continue
		}
		msg := line
		if idx := strings.Index(line, ": "); idx >= 0 {
			msg = line[idx+2:]
		}
		if !seen[msg] {
			seen[msg] = true
			errs = append(errs, msg)
		}
	}
	return errs
}

type CompilerError struct {
	File    string
	Line    int
	Col     int
	Message string
}

func ParseCompilerErrors(output string) []CompilerError {
	var errors []CompilerError
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasSuffix(line, "too many errors") {
			continue
		}
		if strings.HasPrefix(line, "# ") {
			continue
		}

		if parsed, ok := parseCompilerErrorLine(line); ok {
			errors = append(errors, parsed)
		}
	}
	return errors
}

func parseCompilerErrorLine(line string) (CompilerError, bool) {

	idx := strings.Index(line, ": ")
	if idx < 0 {
		return CompilerError{}, false
	}

	locPart := line[:idx]
	msg := line[idx+2:]

	rest, col, ok := splitLastInt(locPart)
	if !ok {
		return CompilerError{}, false
	}
	rest, lineNum, ok := splitLastInt(rest)
	if !ok {
		return CompilerError{}, false
	}

	filePath := rest

	return CompilerError{
		File:    filePath,
		Line:    lineNum,
		Col:     col,
		Message: msg,
	}, true
}

func splitLastInt(s string) (string, int, bool) {
	lastColon := strings.LastIndex(s, ":")
	if lastColon < 0 {
		return "", 0, false
	}
	numStr := s[lastColon+1:]
	n, err := strconv.Atoi(numStr)
	if err != nil {
		return "", 0, false
	}
	return s[:lastColon], n, true
}
