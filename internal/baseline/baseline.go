package baseline

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const DefaultFile = ".gorgon-baseline.json"

type Data struct {
	Score     float64 `json:"score"`
	Killed    int     `json:"killed"`
	Survived  int     `json:"survived"`
	Untested  int     `json:"untested"`
	Total     int     `json:"total"`
	Timestamp string  `json:"timestamp"`
}

func Load(dir, file string) (*Data, error) {
	data, err := os.ReadFile(resolvePath(dir, file))
	if err != nil {
		return nil, err
	}
	var b Data
	return &b, json.Unmarshal(data, &b)
}

func Save(dir, file string, d *Data) error {
	if d.Timestamp == "" {
		d.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	path := resolvePath(dir, file)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// CheckRegression returns an error if current score has dropped below baseline
// by more than tolerance percentage points.
func CheckRegression(current, base *Data, tolerance float64) error {
	if current.Score+tolerance < base.Score {
		return fmt.Errorf("mutation score %.2f%% is below baseline %.2f%% (tolerance: %.2f%%)",
			current.Score, base.Score, tolerance)
	}
	return nil
}

func resolvePath(dir, file string) string {
	if file != "" {
		if filepath.IsAbs(file) {
			return file
		}
		return filepath.Join(dir, file)
	}
	return filepath.Join(dir, DefaultFile)
}
