
package testing

import (
	"fmt"
	"go/parser"
	"go/token"
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


//
//nolint:gocognit,gocyclo,cyclop
func CopyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("failed to stat source: %w", err)
	}
	if !srcInfo.IsDir() {
		return copySingleFile(src, dst)
	}

	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("failed to compute relative path: %w", err)
		}
		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			if info.Name() == ".git" || info.Name() == "vendor" || strings.HasPrefix(info.Name(), "_") {
				return filepath.SkipDir
			}
			return os.MkdirAll(dstPath, info.Mode())
		}

		if !strings.HasSuffix(path, ".go") && path != "go.mod" && path != "go.sum" {
			return nil
		}

		return copyFileWithBuffer(path, dstPath)
	})
}


func extractFilePath(filePath string) string {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, nil, parser.PackageClauseOnly)
	if err == nil && file.Name != nil {
		return file.Name.Name
	}
	return ""
}


func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("failed to read dir %s: %w", src, err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		if err := copyFileWithBuffer(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
			return fmt.Errorf("failed to copy %s: %w", entry.Name(), err)
		}
	}
	return nil
}

func copySingleFile(src, dst string) error {
	return copyFileWithBuffer(src, filepath.Join(dst, filepath.Base(src)))
}

func copyFileWithBuffer(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", src, err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
