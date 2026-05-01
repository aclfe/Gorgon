package reporter

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"

	testing "github.com/aclfe/gorgon/internal/core"
	"github.com/aclfe/gorgon/internal/subconfig"
)

const reportTemplate = `<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<title>Gorgon Mutation Report</title>
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body { font-family: monospace; font-size: 12px; }
.container { display: flex; height: 100vh; }
.sidebar { width: 300px; border-right: 1px solid #ccc; overflow-y: auto; background: #f5f5f5; }
.main { flex: 1; display: flex; flex-direction: column; }
.header { padding: 10px; border-bottom: 1px solid #ccc; background: #fff; }
.stats { display: flex; gap: 20px; }
.stat { display: flex; align-items: baseline; gap: 5px; }
.stat-label { color: #666; }
.stat-value { font-weight: bold; }
.score { font-size: 18px; }
.score.good { color: #2e7d32; }
.score.amber { color: #f57c00; }
.score.bad { color: #c62828; }
.content { flex: 1; overflow-y: auto; background: #fff; }
.tree { padding: 10px; }
.tree-node { cursor: pointer; padding: 2px 0; user-select: none; }
.tree-node:hover { background: #e0e0e0; }
.tree-node.selected { background: #d0d0d0; font-weight: bold; }
.tree-dir { padding-left: 15px; }
.tree-file { padding-left: 15px; }
.tree-toggle { display: inline-block; width: 12px; }
.tree-icon { margin-right: 3px; }
.file-score { float: right; margin-right: 5px; font-size: 10px; }
.file-score.good { color: #2e7d32; }
.file-score.amber { color: #f57c00; }
.file-score.bad { color: #c62828; }
.code-view { padding: 10px; }
.code-line { display: flex; border-bottom: 1px solid #f0f0f0; position: relative; }
.mutation-count { width: 30px; text-align: center; color: #666; font-size: 10px; padding: 2px; cursor: pointer; user-select: none; }
.mutation-count:hover { background: #e0e0e0; }
.line-num { width: 50px; text-align: right; padding-right: 10px; color: #999; user-select: none; border-right: 1px solid #ddd; }
.line-content { flex: 1; padding-left: 10px; white-space: pre; }
.line-killed { background: #c8e6c9; }
.line-survived { background: #ffcdd2; }
.line-timeout { background: #fff9c4; }
.line-error { background: #fff9c4; }
.line-untested { background: #fff9c4; }
.line-none { background: #fff; }
.mutant-popup { display: none; position: absolute; left: 30px; top: 100%; background: #fff; border: 1px solid #999; box-shadow: 2px 2px 8px rgba(0,0,0,0.2); padding: 8px; font-size: 11px; z-index: 1000; min-width: 300px; }
.mutant-popup.show { display: block; }
.mutant-item { padding: 3px 0; border-bottom: 1px solid #eee; }
.mutant-item:last-child { border-bottom: none; }
.mutant-status { display: inline-block; padding: 1px 4px; border-radius: 2px; font-size: 10px; font-weight: bold; margin-right: 5px; }
.mutant-status.killed { background: #c8e6c9; color: #2e7d32; }
.mutant-status.survived { background: #ffcdd2; color: #c62828; }
.mutant-status.timeout { background: #fff9c4; color: #f57c00; }
.mutant-status.error { background: #fff9c4; color: #f57c00; }
.mutant-status.untested { background: #e0e0e0; color: #666; }
</style>
</head>
<body>
<div class="container">
<div class="sidebar">
<div class="tree">
{{template "tree" .Tree}}
</div>
</div>
<div class="main">
<div class="header">
<div class="stats">
<div class="stat">
<span class="stat-label">Score:</span>
<span class="stat-value score {{.ScoreClass}}">{{printf "%.2f" .Stats.Score}}%</span>
</div>
<div class="stat">
<span class="stat-label">Killed:</span>
<span class="stat-value">{{.Stats.Killed}}</span>
</div>
<div class="stat">
<span class="stat-label">Survived:</span>
<span class="stat-value">{{.Stats.Survived}}</span>
</div>
<div class="stat">
<span class="stat-label">Compile Errors:</span>
<span class="stat-value">{{.Stats.CompileErrors}}</span>
</div>
<div class="stat">
<span class="stat-label">Runtime Errors:</span>
<span class="stat-value">{{.Stats.RuntimeErrors}}</span>
</div>
<div class="stat">
<span class="stat-label">Timeout:</span>
<span class="stat-value">{{.Stats.Timeout}}</span>
</div>
<div class="stat">
<span class="stat-label">Untested:</span>
<span class="stat-value">{{.Stats.Untested}}</span>
</div>
<div class="stat">
<span class="stat-label">Invalid:</span>
<span class="stat-value">{{.Stats.Invalid}}</span>
</div>
<div class="stat">
<span class="stat-label">Total:</span>
<span class="stat-value">{{.Stats.Total}}</span>
</div>
</div>
</div>
<div class="content">
<div id="file-view"></div>
</div>
</div>
</div>

{{define "tree"}}
{{range .Children}}
{{if .IsDir}}
<div class="tree-node" onclick="toggleDir(event, this)">
<span class="tree-toggle">▶</span>
<span class="tree-icon">📁</span>
<span>{{.Name}}</span>
</div>
<div class="tree-dir" style="display:none;">
{{template "tree" .}}
</div>
{{else}}
<div class="tree-node tree-file" onclick="showFile('{{.Path}}')">
<span class="tree-toggle"></span>
<span class="tree-icon">📄</span>
<span>{{.Name}}</span>
<span class="file-score {{.ScoreClass}}">{{printf "%.0f" .Score}}%</span>
</div>
{{end}}
{{end}}
{{end}}

<script>
const filesData = {{.Files}};

function toggleDir(e, el) {
e.stopPropagation();
const dir = el.nextElementSibling;
const toggle = el.querySelector('.tree-toggle');
if (dir.style.display === 'none') {
dir.style.display = 'block';
toggle.textContent = '▼';
} else {
dir.style.display = 'none';
toggle.textContent = '▶';
}
}

function showFile(path) {
document.querySelectorAll('.tree-file').forEach(el => el.classList.remove('selected'));
event.target.closest('.tree-file').classList.add('selected');

const fileData = filesData[path];
if (!fileData) return;

let html = '<div class="code-view">';
fileData.Lines.forEach(line => {
const escaped = escapeHtml(line.Content);
const mutantCount = line.Mutants ? line.Mutants.length : 0;
const lineStatus = line.Status;

html += ` + "`" + `<div class="code-line">` + "`" + `;
html += ` + "`" + `<div class="mutation-count" onclick="toggleMutants(event, this)">` + "`" + `;
if (mutantCount > 0) {
html += mutantCount;
}
html += ` + "`" + `</div>` + "`" + `;
html += ` + "`" + `<div class="line-num">${line.Number}</div>` + "`" + `;
html += ` + "`" + `<div class="line-content line-${lineStatus}">${escaped}</div>` + "`" + `;

if (mutantCount > 0) {
html += ` + "`" + `<div class="mutant-popup">` + "`" + `;
line.Mutants.forEach(m => {
html += ` + "`" + `<div class="mutant-item">` + "`" + `;
html += ` + "`" + `<span class="mutant-status ${m.Status}">${m.Status}</span>` + "`" + `;
html += ` + "`" + `#${m.ID} ${m.Operator}` + "`" + `;
if (m.KilledBy) html += ` + "`" + ` → ${m.KilledBy}` + "`" + `;
html += ` + "`" + `</div>` + "`" + `;
});
html += ` + "`" + `</div>` + "`" + `;
}

html += ` + "`" + `</div>` + "`" + `;
});
html += '</div>';
document.getElementById('file-view').innerHTML = html;
}

function toggleMutants(e, el) {
e.stopPropagation();
const popup = el.parentElement.querySelector('.mutant-popup');
if (!popup) return;

document.querySelectorAll('.mutant-popup').forEach(p => {
if (p !== popup) p.classList.remove('show');
});

popup.classList.toggle('show');
}

document.addEventListener('click', (e) => {
if (!e.target.closest('.mutation-count') && !e.target.closest('.mutant-popup')) {
document.querySelectorAll('.mutant-popup').forEach(p => p.classList.remove('show'));
}
});

function escapeHtml(text) {
const div = document.createElement('div');
div.textContent = text;
return div.innerHTML;
}

window.onload = () => {
const firstFile = document.querySelector('.tree-file');
if (firstFile) firstFile.click();
};
</script>
</body>
</html>` + "`"

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
	Stats      ReportStats
	ScoreClass string
	Tree       *TreeNode
	Files      map[string]*FileData
}

func writeHTMLReport(mutants []testing.Mutant, stats ReportStats, threshold float64, resolver *subconfig.Resolver, outputFile string) error {
	if outputFile == "" {
		return fmt.Errorf("output file path is required for HTML format")
	}

	byFile := GroupMutantsByFile(mutants)

	scoreClass := ScoreClass(stats.Score, threshold)

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
				hasTimeout := false

				for _, m := range mutantsOnLine {
					lineStatuses[i].Mutants = append(lineStatuses[i].Mutants, MutantInfo{
						ID:       m.ID,
						Operator: m.Operator.Name(),
						Status:   m.Status,
						KilledBy: m.KilledBy,
					})

					switch m.Status {
					case testing.StatusSurvived:
						hasSurvived = true
						allKilled = false
					case testing.StatusUntested:
						hasUntested = true
						allKilled = false
					case testing.StatusTimeout:
						hasTimeout = true
						allKilled = false
					case testing.StatusError:
						hasError = true
						allKilled = false
					case testing.StatusKilled:
						// OK
					default:
						allKilled = false
					}
				}

				// Priority: survived > timeout > untested > error > killed
				if hasSurvived {
					lineStatuses[i].Status = testing.StatusSurvived
				} else if hasTimeout {
					lineStatuses[i].Status = testing.StatusTimeout
				} else if hasUntested {
					lineStatuses[i].Status = testing.StatusUntested
				} else if hasError {
					lineStatuses[i].Status = testing.StatusError
				} else if allKilled {
					lineStatuses[i].Status = testing.StatusKilled
				}
			}
		}

		fileKilled, fileSurvived, fileUntested, fileTimeout := 0, 0, 0, 0
		for _, mutantsOnLine := range lineMutants {
			for _, m := range mutantsOnLine {
				switch m.Status {
				case testing.StatusKilled:
					fileKilled++
				case testing.StatusSurvived:
					fileSurvived++
				case testing.StatusUntested:
					fileUntested++
				case testing.StatusTimeout:
					fileTimeout++
				}
			}
		}

		fileScore := CalculateScore(fileKilled, fileSurvived, fileUntested, fileTimeout)
		fileThreshold := threshold
		if resolver != nil {
			fileThreshold = resolver.EffectiveThreshold(filePath, threshold)
		}

		fileScoreClass := ScoreClass(fileScore, fileThreshold)

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
		Stats:      stats,
		ScoreClass: scoreClass,
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
