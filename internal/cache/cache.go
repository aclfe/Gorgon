package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

type Entry struct {
	Status string `json:"status"`
}

type Cache struct {
	Entries map[string]Entry `json:"entries"`
}

func New() *Cache {
	return &Cache{
		Entries: make(map[string]Entry),
	}
}

func cacheDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home dir: %w", err)
	}
	return filepath.Join(home, ".cache", "gorgon"), nil
}

func cachePath(projectDir string) (string, error) {
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(projectDir)
	if err != nil {
		return "", err
	}
	name := filepath.Base(abs) + "_cache_gorgon.json"
	return filepath.Join(dir, name), nil
}

func Path(projectDir string) (string, error) {
	return cachePath(projectDir)
}

func Load(projectDir string) (*Cache, error) {
	path, err := cachePath(projectDir)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return New(), nil
		}
		return nil, fmt.Errorf("failed to read cache: %w", err)
	}

	var c Cache
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("failed to parse cache: %w", err)
	}
	return &c, nil
}

func (c *Cache) Save(projectDir string) error {
	dir, err := cacheDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create cache dir: %w", err)
	}

	path, err := cachePath(projectDir)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("failed to write cache: %w", err)
	}
	return nil
}

func (c *Cache) Key(filePath string, line, col int, nodeType uint8, operator string, fileHash string) string {
	// Format: "filePath:line:col:nodeType:operator:fileHash"
	const colon = byte(':')
	cap := len(filePath) + len(operator) + len(fileHash) + 30
	buf := make([]byte, 0, cap)
	buf = append(buf, filePath...)
	buf = append(buf, colon)
	buf = strconv.AppendInt(buf, int64(line), 10)
	buf = append(buf, colon)
	buf = strconv.AppendInt(buf, int64(col), 10)
	buf = append(buf, colon)
	buf = strconv.AppendUint(buf, uint64(nodeType), 10)
	buf = append(buf, colon)
	buf = append(buf, operator...)
	buf = append(buf, colon)
	buf = append(buf, fileHash...)

	h := sha256.New()
	h.Write(buf)
	return hex.EncodeToString(h.Sum(nil))
}

func (c *Cache) Get(key string) (Entry, bool) {
	e, ok := c.Entries[key]
	return e, ok
}

func (c *Cache) Set(key string, status string) {
	c.Entries[key] = Entry{Status: status}
}
