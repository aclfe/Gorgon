package diff

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// FileLines maps absolute file paths to sets of changed line numbers.
type FileLines map[string]map[int]bool

// Resolve returns changed lines from either a git ref (e.g. "HEAD~1", "HEAD",
// "--cached") or a path to a .patch file on disk.
// Returns nil, nil if ref is empty — callers treat nil as "no filter".
func Resolve(ref string) (FileLines, error) {
	if ref == "" {
		return nil, nil
	}
	// If it looks like a file path that exists, treat it as a patch file.
	if _, err := os.Stat(ref); err == nil {
		f, err := os.Open(ref)
		if err != nil {
			return nil, fmt.Errorf("open patch file %q: %w", ref, err)
		}
		defer f.Close()
		return parse(f)
	}
	return fromGit(ref)
}

func fromGit(ref string) (FileLines, error) {
	// --unified=0 gives us only changed lines with no context,
	// which is exactly what we need — no risk of false positives
	// from nearby unchanged lines.
	cmd := exec.Command("git", "diff", ref, "--unified=0")
	out, err := cmd.Output()
	if err != nil {
		// Exit code 1 from git diff just means "differences found" — not an error.
		// Any real failure (not a git repo, bad ref) shows up in stderr.
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
			return nil, fmt.Errorf("git diff %s: %s", ref, strings.TrimSpace(string(exitErr.Stderr)))
		}
	}
	return parse(strings.NewReader(string(out)))
}

// parse reads a unified diff and returns the set of added/modified
// lines per file. Only "+" lines are tracked — removed lines no
// longer exist in the working tree so there's nothing to mutate.
func parse(r io.Reader) (FileLines, error) {
	result := make(FileLines)
	var currentFile string
	var newLine int // current line number in the new file

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()

		switch {
		// "+++ b/path/to/file.go"
		case strings.HasPrefix(line, "+++ "):
			path := strings.TrimPrefix(line, "+++ ")
			path = strings.TrimPrefix(path, "b/")
			if path == "/dev/null" {
				currentFile = ""
				continue
			}
			abs, err := filepath.Abs(path)
			if err != nil {
				abs = path
			}
			currentFile = abs
			if result[currentFile] == nil {
				result[currentFile] = make(map[int]bool)
			}

		// "@@ -old,count +new,count @@"
		case strings.HasPrefix(line, "@@ "):
			newLine, _ = parseHunkHeader(line)

		// Added line — record it and advance
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			if currentFile != "" {
				result[currentFile][newLine] = true
			}
			newLine++

		// Removed line — doesn't exist in new file, don't advance newLine
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			// intentionally blank

		// Context line (or diff header) — advance newLine
		default:
			if currentFile != "" && !strings.HasPrefix(line, "\\") {
				newLine++
			}
		}
	}
	return result, scanner.Err()
}

// parseHunkHeader extracts the new-file start line from "@@ -a,b +c,d @@".
// With --unified=0 the count is always 0 or 1, but we parse it anyway
// for correctness when called with patch files generated differently.
func parseHunkHeader(line string) (start, count int) {
	// Find the "+c" or "+c,d" segment
	plusIdx := strings.Index(line[3:], "+")
	if plusIdx < 0 {
		return 0, 0
	}
	chunk := line[3+plusIdx+1:]
	if spaceIdx := strings.IndexByte(chunk, ' '); spaceIdx >= 0 {
		chunk = chunk[:spaceIdx]
	}
	parts := strings.SplitN(chunk, ",", 2)
	start, _ = strconv.Atoi(parts[0])
	if len(parts) == 2 {
		count, _ = strconv.Atoi(parts[1])
	} else {
		count = 1
	}
	return start, count
}
