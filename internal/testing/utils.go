// Package testing provides testing utilities for the gorgon project.
package testing

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// CopyDir copies a directory recursively.
// If src is a single file, it copies that file to dst directory.
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

	// Handle directory case
	if err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Calculate relative path and destination path
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("failed to compute relative path: %w", err)
		}
		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			if info.Name() == ".git" || info.Name() == "vendor" || strings.HasPrefix(info.Name(), "_") {
				return filepath.SkipDir
			}
			if err := os.MkdirAll(dstPath, info.Mode()); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
			return nil
		}

		if !strings.HasSuffix(path, ".go") && path != "go.mod" && path != "go.sum" {
			return nil
		}
		//nolint:gosec // Walking directory and opening files
		srcFile, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open file %s: %w", path, err)
		}
		defer func() {
			//nolint:errcheck
			_ = srcFile.Close() // ignore error from Close
		}()
		//nolint:gosec // Creating file in destination
		dstFile, err := os.Create(dstPath)
		if err != nil {
			return fmt.Errorf("failed to create file %s: %w", dstPath, err)
		}
		defer func() {
			_ = dstFile.Close() // ignore error from Close
		}()
		if _, err = io.Copy(dstFile, srcFile); err != nil {
			return fmt.Errorf("failed to copy content to %s: %w", dstPath, err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("walk failed: %w", err)
	}
	return nil
}

func copySingleFile(src, dst string) error {
	//nolint:gosec // Copying user-provided file
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer func() {
		_ = srcFile.Close() // ignore error from Close
	}()

	dstPath := filepath.Join(dst, filepath.Base(src))
	//nolint:gosec // Creating user-provided file
	dstFile, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("failed to create dest file: %w", err)
	}
	defer func() {
		_ = dstFile.Close() // ignore error from Close
	}()

	if _, err = io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("failed to copy file content: %w", err)
	}
	return nil
}

func findGoMod(dir string) string {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "" // best effort? or should we log?
	}

	for {
		goModPath := filepath.Join(absDir, "go.mod")
		if fileExists(goModPath) {
			return goModPath
		}

		parent := filepath.Dir(absDir)
		if parent == absDir {
			break
		}
		absDir = parent
	}
	return ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
