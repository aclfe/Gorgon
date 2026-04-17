package reporter

import (
	_ "embed"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aclfe/gorgon/internal/core"
	"github.com/aclfe/gorgon/internal/subconfig"
)

//go:embed report.html
var reportTemplate string

type LineCoverage struct {
	Number  int
	Content string
	Status  string
	Mutants []MutantInfo
}

type MutantInfo struct {
	ID       int
	Operator string
	Status   string
	KilledBy string
}

type FileData struct {
	Path       string
	RelPath    string
	Lines      []LineCoverage
	Score      float64
	ScoreClass string
}

type TreeNode struct {
	Name       string
	Path       string
	IsDir      bool
	Children   []*TreeNode
	Score      float64
	ScoreClass string
}

type ReportData struct {
	Score      float64
	ScoreClass string
	Killed     int
	Survived   int
	Errors     int
	Untested   int
	Total      int
	Tree       *TreeNode
	Files      map[string]*FileData
}

func writeHTMLReport(mutants []testing.Mutant, totalMutants int, threshold float64, resolver *subconfig.Resolver, outputFile string) error {
	if outputFile == "" {
		return fmt.Errorf("output file path is required for HTML format")
	}

	byFile := make(map[string]map[int][]testing.Mutant)
	for _, m := range mutants {
		if m.Site.File == nil {
			continue
		}
		filePath := m.Site.File.Name()
		if byFile[filePath] == nil {
			byFile[filePath] = make(map[int][]testing.Mutant)
		}
		byFile[filePath][m.Site.Line] = append(byFile[filePath][m.Site.Line], m)
	}

	killed, survived, errors, untested := 0, 0, 0, 0
	for _, m := range mutants {
		switch m.Status {
		case "killed":
			killed++
		case "survived":
			survived++
		case "error":
			errors++
		case "untested":
			untested++
		}
	}

	effectiveTotal := killed + survived + untested
	score := 0.0
	if effectiveTotal > 0 {
		score = float64(killed) / float64(effectiveTotal) * 100
	}

	scoreClass := "good"
	if score < threshold {
		scoreClass = "bad"
	} else if score < 80 {
		scoreClass = "amber"
	}

	cwd, _ := os.Getwd()
	filesData := make(map[string]*FileData)
	for filePath, lineMutants := range byFile {
		content, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		lines := strings.Split(string(content), "\n")
		lineStatuses := make([]LineCoverage, len(lines))

		for i, lineContent := range lines {
			lineNum := i + 1
			lineStatuses[i] = LineCoverage{
				Number:  lineNum,
				Content: lineContent,
				Status:  "none",
			}

			if mutantsOnLine, ok := lineMutants[lineNum]; ok {
				allKilled := true
				hasSurvived := false
				hasError := false
				hasUntested := false

				for _, m := range mutantsOnLine {
					lineStatuses[i].Mutants = append(lineStatuses[i].Mutants, MutantInfo{
						ID:       m.ID,
						Operator: m.Operator.Name(),
						Status:   m.Status,
						KilledBy: m.KilledBy,
					})

					if m.Status == "survived" {
						hasSurvived = true
						allKilled = false
					} else if m.Status == "untested" {
						hasUntested = true
						allKilled = false
					} else if m.Status == "error" {
						hasError = true
						allKilled = false
					} else if m.Status != "killed" {
						allKilled = false
					}
				}

				// Priority: survived > untested > error > killed
				if hasSurvived {
					lineStatuses[i].Status = "survived"
				} else if hasUntested {
					lineStatuses[i].Status = "untested"
				} else if hasError {
					lineStatuses[i].Status = "error"
				} else if allKilled {
					lineStatuses[i].Status = "killed"
				}
			}
		}

		fileKilled, fileSurvived, fileUntested := 0, 0, 0
		for _, mutantsOnLine := range lineMutants {
			for _, m := range mutantsOnLine {
				switch m.Status {
				case "killed":
					fileKilled++
				case "survived":
					fileSurvived++
				case "untested":
					fileUntested++
				}
			}
		}

		fileEffective := fileKilled + fileSurvived + fileUntested
		fileScore := 0.0
		if fileEffective > 0 {
			fileScore = float64(fileKilled) / float64(fileEffective) * 100
		}

		fileScoreClass := "good"
		if fileScore < threshold {
			fileScoreClass = "bad"
		} else if fileScore < 80 {
			fileScoreClass = "amber"
		}

		relPath := filePath
		if cwd != "" {
			if rel, err := filepath.Rel(cwd, filePath); err == nil {
				relPath = rel
			}
		}
		filesData[filePath] = &FileData{
			Path:       filePath,
			RelPath:    relPath,
			Lines:      lineStatuses,
			Score:      fileScore,
			ScoreClass: fileScoreClass,
		}
	}

	tree := buildTree(filesData)

	data := ReportData{
		Score:      score,
		ScoreClass: scoreClass,
		Killed:     killed,
		Survived:   survived,
		Errors:     errors,
		Untested:   untested,
		Total:      totalMutants,
		Tree:       tree,
		Files:      filesData,
	}

	if err := os.MkdirAll(outputFile, 0o755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	tmpl := template.Must(template.New("report").Parse(reportTemplate))
	indexPath := filepath.Join(outputFile, "index.html")
	f, err := os.Create(indexPath)
	if err != nil {
		return fmt.Errorf("failed to create index.html: %w", err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, data); err != nil {
		return fmt.Errorf("failed to write index.html: %w", err)
	}

	return nil
}

func buildTree(filesData map[string]*FileData) *TreeNode {
	root := &TreeNode{Name: "root", IsDir: true, Children: []*TreeNode{}}

	for _, fileData := range filesData {
		parts := strings.Split(fileData.RelPath, string(filepath.Separator))
		current := root

		for i, part := range parts {
			isLast := i == len(parts)-1
			var child *TreeNode
			for _, c := range current.Children {
				if c.Name == part {
					child = c
					break
				}
			}

			if child == nil {
				child = &TreeNode{
					Name:     part,
					Path:     fileData.Path,
					IsDir:    !isLast,
					Children: []*TreeNode{},
				}
				if isLast {
					child.Score = fileData.Score
					child.ScoreClass = fileData.ScoreClass
				}
				current.Children = append(current.Children, child)
			}
			current = child
		}
	}

	sortTree(root)
	return root
}

func sortTree(node *TreeNode) {
	sort.Slice(node.Children, func(i, j int) bool {
		if node.Children[i].IsDir != node.Children[j].IsDir {
			return node.Children[i].IsDir
		}
		return node.Children[i].Name < node.Children[j].Name
	})
	for _, child := range node.Children {
		if child.IsDir {
			sortTree(child)
		}
	}
}
