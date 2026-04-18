package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/aclfe/gorgon/pkg/config"
)

func TestParseBasicFlags(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		check    func(*Flags, error)
	}{
		{
			name: "empty args",
			args: []string{},
			check: func(f *Flags, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if f.PkgPath != "." {
					t.Errorf("expected PkgPath='.', got '%s'", f.PkgPath)
				}
				if f.Operators != "all" {
					t.Errorf("expected Operators='all', got '%s'", f.Operators)
				}
			},
		},
		{
			name: "single path",
			args: []string{"./path/to/pkg"},
			check: func(f *Flags, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(f.Targets) != 1 || f.Targets[0] != "./path/to/pkg" {
					t.Errorf("expected Targets=['./path/to/pkg'], got %v", f.Targets)
				}
			},
		},
		{
			name: "multiple paths",
			args: []string{"./path1", "./path2"},
			check: func(f *Flags, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(f.Targets) != 2 {
					t.Errorf("expected 2 targets, got %d", len(f.Targets))
				}
			},
		},
		{
			name: "print-ast flag",
			args: []string{"-print-ast", "./path"},
			check: func(f *Flags, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !f.PrintAST {
					t.Error("expected PrintAST=true")
				}
			},
		},
		{
			name: "operators flag",
			args: []string{"-operators=arithmetic_flip,condition_negation", "./path"},
			check: func(f *Flags, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if f.Operators != "arithmetic_flip,condition_negation" {
					t.Errorf("expected Operators='arithmetic_flip,condition_negation', got '%s'", f.Operators)
				}
			},
		},
		{
			name: "concurrent flag",
			args: []string{"-concurrent=half", "./path"},
			check: func(f *Flags, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if f.Concurrent != "half" {
					t.Errorf("expected Concurrent='half', got '%s'", f.Concurrent)
				}
			},
		},
		{
			name: "threshold flag",
			args: []string{"-threshold=80", "./path"},
			check: func(f *Flags, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if f.Threshold != 80 {
					t.Errorf("expected Threshold=80, got %f", f.Threshold)
				}
			},
		},
		{
			name: "threshold boundary 0",
			args: []string{"-threshold=0", "./path"},
			check: func(f *Flags, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if f.Threshold != 0 {
					t.Errorf("expected Threshold=0, got %f", f.Threshold)
				}
			},
		},
		{
			name: "threshold boundary 100",
			args: []string{"-threshold=100", "./path"},
			check: func(f *Flags, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if f.Threshold != 100 {
					t.Errorf("expected Threshold=100, got %f", f.Threshold)
				}
			},
		},
		{
			name: "cache flag",
			args: []string{"-cache", "./path"},
			check: func(f *Flags, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !f.UseCache {
					t.Error("expected UseCache=true")
				}
			},
		},
		{
			name: "dry-run flag",
			args: []string{"-dry-run", "./path"},
			check: func(f *Flags, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !f.DryRun {
					t.Error("expected DryRun=true")
				}
			},
		},
		{
			name: "debug flag",
			args: []string{"-debug", "./path"},
			check: func(f *Flags, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !f.Debug {
					t.Error("expected Debug=true")
				}
			},
		},
		{
			name: "progbar flag",
			args: []string{"-progbar", "./path"},
			check: func(f *Flags, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !f.ProgBar {
					t.Error("expected ProgBar=true")
				}
			},
		},
		{
			name: "show-killed flag",
			args: []string{"-show-killed", "./path"},
			check: func(f *Flags, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !f.ShowKilled {
					t.Error("expected ShowKilled=true")
				}
			},
		},
		{
			name: "show-survived flag",
			args: []string{"-show-survived", "./path"},
			check: func(f *Flags, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !f.ShowSurvived {
					t.Error("expected ShowSurvived=true")
				}
			},
		},
		{
			name: "diff flag",
			args: []string{"-diff=HEAD~1", "./path"},
			check: func(f *Flags, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if f.Diff != "HEAD~1" {
					t.Errorf("expected Diff='HEAD~1', got '%s'", f.Diff)
				}
			},
		},
		{
			name: "config flag",
			args: []string{"-config=gorgon.yml", "./path"},
			check: func(f *Flags, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if f.ConfigFile != "gorgon.yml" {
					t.Errorf("expected ConfigFile='gorgon.yml', got '%s'", f.ConfigFile)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := Parse(tt.args)
			tt.check(f, err)
		})
	}
}

func TestParseConcurrent(t *testing.T) {
	numCPU := runtime.NumCPU()
	tests := []struct {
		input    string
		expected int
	}{
		{"all", numCPU},
		{"half", numCPU / 2},
		{"1", 1},
		{"2", 2},
		{"4", 4},
		{"invalid", numCPU},
		{"0", numCPU},
		{"-1", numCPU},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := ParseConcurrent(tt.input)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name     string
		flags    *Flags
		check    func(*config.Config, error)
	}{
		{
			name: "default config",
			flags: &Flags{},
			check: func(cfg *config.Config, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			},
		},
		{
			name: "operators override",
			flags: &Flags{Operators: "arithmetic_flip,condition_negation"},
			check: func(cfg *config.Config, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(cfg.Operators) != 2 {
					t.Errorf("expected 2 operators, got %d", len(cfg.Operators))
				}
			},
		},
		{
			name: "concurrent override",
			flags: &Flags{Concurrent: "half"},
			check: func(cfg *config.Config, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if cfg.Concurrent != "half" {
					t.Errorf("expected Concurrent='half', got '%s'", cfg.Concurrent)
				}
			},
		},
		{
			name: "threshold override",
			flags: &Flags{Threshold: 80.5},
			check: func(cfg *config.Config, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if cfg.Threshold != 80.5 {
					t.Errorf("expected Threshold=80.5, got %f", cfg.Threshold)
				}
			},
		},
		{
			name: "cache override",
			flags: &Flags{UseCache: true},
			check: func(cfg *config.Config, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !cfg.Cache {
					t.Error("expected Cache=true")
				}
			},
		},
		{
			name: "dry-run override",
			flags: &Flags{DryRun: true},
			check: func(cfg *config.Config, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !cfg.DryRun {
					t.Error("expected DryRun=true")
				}
			},
		},
		{
			name: "diff override",
			flags: &Flags{Diff: "HEAD~1"},
			check: func(cfg *config.Config, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if cfg.Diff != "HEAD~1" {
					t.Errorf("expected Diff='HEAD~1', got '%s'", cfg.Diff)
				}
			},
		},
		{
			name: "debug override",
			flags: &Flags{Debug: true},
			check: func(cfg *config.Config, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !cfg.Debug {
					t.Error("expected Debug=true")
				}
			},
		},
		{
			name: "progbar override",
			flags: &Flags{ProgBar: true},
			check: func(cfg *config.Config, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !cfg.ProgBar {
					t.Error("expected ProgBar=true")
				}
			},
		},
		{
			name: "show-killed override",
			flags: &Flags{ShowKilled: true},
			check: func(cfg *config.Config, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !cfg.ShowKilled {
					t.Error("expected ShowKilled=true")
				}
			},
		},
		{
			name: "show-survived override",
			flags: &Flags{ShowSurvived: true},
			check: func(cfg *config.Config, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !cfg.ShowSurvived {
					t.Error("expected ShowSurvived=true")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := tt.flags.LoadConfig()
			tt.check(cfg, err)
		})
	}
}

func TestValidateChecks(t *testing.T) {
	tests := []struct {
		name     string
		flags    *Flags
		wantErr  bool
		errMsg   string
	}{
		{
			name:    "no config file - no error",
			flags:   &Flags{},
			wantErr: false,
		},
		{
			name:    "config file with other flags - error",
			flags:   &Flags{ConfigFile: "config.yml", Operators: "arithmetic_flip"},
			wantErr: true,
		},
		{
			name:    "config file only - no error",
			flags:   &Flags{ConfigFile: "config.yml", PkgPath: ".", Operators: "all", Concurrent: "all", Threshold: 0},
			wantErr: false, // Only error if ConfigFile AND another flag is set
		},
		{
			name:    "config file with threshold - error",
			flags:   &Flags{ConfigFile: "config.yml", Threshold: 80},
			wantErr: true,
		},
		{
			name:    "config file with cache - error",
			flags:   &Flags{ConfigFile: "config.yml", UseCache: true},
			wantErr: true,
		},
		{
			name:    "config file with debug - error",
			flags:   &Flags{ConfigFile: "config.yml", Debug: true},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.flags.ValidateChecks()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateChecks() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil && tt.errMsg != "" && err.Error() != tt.errMsg {
				t.Errorf("expected error message '%s', got '%s'", tt.errMsg, err.Error())
			}
		})
	}
}

func TestComplexFlagCombinations(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		check    func(*Flags, error)
	}{
		{
			name: "complex combination 1",
			args: []string{
				"-operators=arithmetic_flip,condition_negation",
				"-concurrent=2",
				"-threshold=80",
				"-cache",
				"-debug",
				"-show-killed",
				"-diff=HEAD~1",
				"./path/to/pkg",
			},
			check: func(f *Flags, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if f.Operators != "arithmetic_flip,condition_negation" {
					t.Errorf("expected Operators='arithmetic_flip,condition_negation', got '%s'", f.Operators)
				}
				if f.Concurrent != "2" {
					t.Errorf("expected Concurrent='2', got '%s'", f.Concurrent)
				}
				if f.Threshold != 80 {
					t.Errorf("expected Threshold=80, got %f", f.Threshold)
				}
				if !f.UseCache {
					t.Error("expected UseCache=true")
				}
				if !f.Debug {
					t.Error("expected Debug=true")
				}
				if !f.ShowKilled {
					t.Error("expected ShowKilled=true")
				}
				if f.Diff != "HEAD~1" {
					t.Errorf("expected Diff='HEAD~1', got '%s'", f.Diff)
				}
			},
		},
		{
			name: "complex combination with threshold",
			args: []string{
				"-threshold=70",
				"-concurrent=half",
				"-cache",
				"./path",
			},
			check: func(f *Flags, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if f.Threshold != 70 {
					t.Errorf("expected Threshold=70, got %f", f.Threshold)
				}
				if f.Concurrent != "half" {
					t.Errorf("expected Concurrent='half', got '%s'", f.Concurrent)
				}
				if !f.UseCache {
					t.Error("expected UseCache=true")
				}
			},
		},
		{
			name: "all boolean flags",
			args: []string{
				"-print-ast",
				"-cache",
				"-dry-run",
				"-debug",
				"-progbar",
				"-show-killed",
				"-show-survived",
				"./path",
			},
			check: func(f *Flags, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !f.PrintAST {
					t.Error("expected PrintAST=true")
				}
				if !f.UseCache {
					t.Error("expected UseCache=true")
				}
				if !f.DryRun {
					t.Error("expected DryRun=true")
				}
				if !f.Debug {
					t.Error("expected Debug=true")
				}
				if !f.ProgBar {
					t.Error("expected ProgBar=true")
				}
				if !f.ShowKilled {
					t.Error("expected ShowKilled=true")
				}
				if !f.ShowSurvived {
					t.Error("expected ShowSurvived=true")
				}
			},
		},
		{
			name: "diff with operators",
			args: []string{
				"-diff=HEAD~1",
				"-operators=arithmetic_flip",
				"./path",
			},
			check: func(f *Flags, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if f.Diff != "HEAD~1" {
					t.Errorf("expected Diff='HEAD~1', got '%s'", f.Diff)
				}
				if f.Operators != "arithmetic_flip" {
					t.Errorf("expected Operators='arithmetic_flip', got '%s'", f.Operators)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := Parse(tt.args)
			tt.check(f, err)
		})
	}
}

func TestLoadConfigWithComplexFlags(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yml")

	configContent := `
operators:
  - arithmetic_flip
  - condition_negation
concurrent: 2
threshold: 80
cache: true
dry_run: false
debug: true
progbar: true
show_killed: true
show_survived: true
exclude:
  - "*_test.go"
include:
  - "internal/"
skip:
  - "vendor/"
skip_func:
  - "foo/bar.go:MyFunc"
tests:
  - "cmd/gorgon/main_test.go"
diff: "HEAD~1"
outputs:
  - "textfile:report.txt"
  - "junit:results.xml"
cpu_profile: "profile.out"
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	tests := []struct {
		name     string
		args     []string
		check    func(*config.Config, error)
	}{
		{
			name: "load config from file",
			args: []string{"-config=" + configPath},
			check: func(cfg *config.Config, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if len(cfg.Operators) != 2 {
					t.Errorf("expected 2 operators, got %d", len(cfg.Operators))
				}
				if cfg.Concurrent != "2" {
					t.Errorf("expected Concurrent='2', got '%s'", cfg.Concurrent)
				}
				if cfg.Threshold != 80 {
					t.Errorf("expected Threshold=80, got %f", cfg.Threshold)
				}
				if !cfg.Cache {
					t.Error("expected Cache=true")
				}
				if !cfg.Debug {
					t.Error("expected Debug=true")
				}
				if !cfg.ProgBar {
					t.Error("expected ProgBar=true")
				}
				if !cfg.ShowKilled {
					t.Error("expected ShowKilled=true")
				}
				if !cfg.ShowSurvived {
					t.Error("expected ShowSurvived=true")
				}
				if len(cfg.Exclude) != 1 {
					t.Errorf("expected 1 exclude pattern, got %d", len(cfg.Exclude))
				}
				if len(cfg.Include) != 1 {
					t.Errorf("expected 1 include pattern, got %d", len(cfg.Include))
				}
				if len(cfg.Skip) != 1 {
					t.Errorf("expected 1 skip pattern, got %d", len(cfg.Skip))
				}
				if len(cfg.SkipFunc) != 1 {
					t.Errorf("expected 1 skip-func, got %d", len(cfg.SkipFunc))
				}
				if len(cfg.Tests) != 1 {
					t.Errorf("expected 1 test path, got %d", len(cfg.Tests))
				}
				if cfg.Diff != "HEAD~1" {
					t.Errorf("expected Diff='HEAD~1', got '%s'", cfg.Diff)
				}
				if len(cfg.Outputs) != 2 {
					t.Errorf("expected 2 outputs, got %d", len(cfg.Outputs))
				}
				if cfg.CPUProfile != "profile.out" {
					t.Errorf("expected CPUProfile='profile.out', got '%s'", cfg.CPUProfile)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := Parse(tt.args)
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			cfg, err := f.LoadConfig()
			tt.check(cfg, err)
		})
	}
}

func TestParseOperators(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *config.Config
		wantErr     bool
		errContains string
		allowEmpty  bool
	}{
		{
			name: "empty operators list",
			cfg:  &config.Config{Operators: []string{}},
			wantErr: false,
			allowEmpty: true,
		},
		{
			name: "all operator",
			cfg:  &config.Config{Operators: []string{"all"}},
			wantErr: false,
			allowEmpty: true, // May be empty if no operators registered
		},
		{
			name: "unknown operator",
			cfg:  &config.Config{Operators: []string{"unknown_operator"}},
			wantErr: true,
			errContains: "unknown operator",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ops, err := ParseOperators(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseOperators() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil && tt.errContains != "" {
				if !contains(err.Error(), tt.errContains) {
					t.Errorf("expected error to contain '%s', got '%s'", tt.errContains, err.Error())
				}
			}
			if !tt.wantErr && !tt.allowEmpty && len(ops) == 0 {
				t.Errorf("expected operators, got none for config: %v", tt.cfg.Operators)
			}
		})
	}
}

func TestSplitAndTrim(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"foo,bar", []string{"foo", "bar"}},
		{" foo , bar ", []string{"foo", "bar"}},
		{"foo,,bar", []string{"foo", "bar"}},
		{" ", []string{}},
		{"", []string{}},
		{"foo", []string{"foo"}},
		{"a,b,c,d", []string{"a", "b", "c", "d"}},
		{"  a  ,  b  ,  c  ", []string{"a", "b", "c"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := splitAndTrim(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result)
			} else {
				for i := range result {
					if result[i] != tt.expected[i] {
						t.Errorf("expected %v, got %v", tt.expected, result)
						break
					}
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
