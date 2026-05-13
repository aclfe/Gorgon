// +build integration

package integration

import (
	"encoding/json"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aclfe/gorgon/internal/badge"
)

// ============================================================================
// BADGE — JSON GENERATION
// ============================================================================

// TestBadge_GenerateJSON_Structure verifies the JSON output contains all four
// required shields.io fields: schemaVersion, label, message, and color.
func TestBadge_GenerateJSON_Structure(t *testing.T) {
	out, err := badge.GenerateJSON(85.0)
	if err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}

	var fields map[string]interface{}
	if err := json.Unmarshal([]byte(out), &fields); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	if v, ok := fields["schemaVersion"]; !ok {
		t.Error("missing field: schemaVersion")
	} else if v.(float64) != 1 {
		t.Errorf("schemaVersion: want 1, got %v", v)
	}
	if _, ok := fields["label"]; !ok {
		t.Error("missing field: label")
	}
	if _, ok := fields["message"]; !ok {
		t.Error("missing field: message")
	}
	if _, ok := fields["color"]; !ok {
		t.Error("missing field: color")
	}
}

// TestBadge_GenerateJSON_LabelIsMutation verifies the label is always "mutation".
func TestBadge_GenerateJSON_LabelIsMutation(t *testing.T) {
	out, err := badge.GenerateJSON(42.5)
	if err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}

	var fields map[string]interface{}
	if err := json.Unmarshal([]byte(out), &fields); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if fields["label"] != "mutation" {
		t.Errorf("label: want 'mutation', got %q", fields["label"])
	}
}

// TestBadge_GenerateJSON_ScoreFormatting verifies the message field formats
// the score as "XX.X%" with one decimal place.
func TestBadge_GenerateJSON_ScoreFormatting(t *testing.T) {
	tests := []struct {
		score   float64
		wantMsg string
	}{
		{0, "0.0%"},
		{50, "50.0%"},
		{85.3, "85.3%"},
		{99.9, "99.9%"},
		{100, "100.0%"},
		{3.7, "3.7%"},
	}

	for _, tt := range tests {
		out, err := badge.GenerateJSON(tt.score)
		if err != nil {
			t.Fatalf("GenerateJSON(%.1f): %v", tt.score, err)
		}
		var fields map[string]interface{}
		json.Unmarshal([]byte(out), &fields)
		if fields["message"] != tt.wantMsg {
			t.Errorf("score %.1f: want message %q, got %q", tt.score, tt.wantMsg, fields["message"])
		}
	}
}

// TestBadge_GenerateJSON_ScoreRounding verifies that scores round to one
// decimal place consistently. Uses values that are exactly representable
// in float64 to avoid IEEE 754 precision artifacts (e.g. 85.55 is
// actually 85.54999... in float64 and rounds down, not up).
func TestBadge_GenerateJSON_ScoreRounding(t *testing.T) {
	tests := []struct {
		score   float64
		wantMsg string
	}{
		{85.5, "85.5%"},
		{85.0, "85.0%"},
		{0.0, "0.0%"},
		{0.125, "0.1%"}, // .125 is exactly representable, rounds down
		{0.75, "0.8%"},  // .75 is exactly representable, rounds up
		{99.875, "99.9%"},
	}

	for _, tt := range tests {
		out, err := badge.GenerateJSON(tt.score)
		if err != nil {
			t.Fatalf("GenerateJSON(%.3f): %v", tt.score, err)
		}
		var fields map[string]interface{}
		json.Unmarshal([]byte(out), &fields)
		if fields["message"] != tt.wantMsg {
			t.Errorf("score %.3f: want message %q, got %q", tt.score, tt.wantMsg, fields["message"])
		}
	}
}

// TestBadge_GenerateJSON_ScoreRoundingFloatingPoint documents that values
// like 85.55 which can't be exactly represented in float64 (IEEE 754)
// produce a correctly rounded result given the actual stored value.
// This test exists to catch any accidental change to the formatting logic.
func TestBadge_GenerateJSON_ScoreRoundingFloatingPoint(t *testing.T) {
	// 85.55 in float64 is ~85.54999..., so %.1f correctly produces 85.5
	out, err := badge.GenerateJSON(85.55)
	if err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}
	var fields map[string]interface{}
	json.Unmarshal([]byte(out), &fields)

	// This is the correct behavior for float64 representation of 85.55
	if fields["message"] != "85.5%" {
		t.Errorf("float64(85.55): got %q — if this changed, review formatting logic", fields["message"])
	}
}

// ============================================================================
// BADGE — COLOR MAPPING
//
// Color thresholds (from internal/badge/badge.go):
//   >= 80: #4c1       (bright green)
//   >= 60: #97ca00    (yellow-green)
//   >= 40: #dfb317    (yellow)
//   >= 20: #fe7d37    (orange)
//    < 20: #e05d44    (red)
// ============================================================================

// TestBadge_GenerateJSON_ColorBoundaries verifies the color field at each
// threshold boundary, including values just above and below to catch
// off-by-one errors.
func TestBadge_GenerateJSON_ColorBoundaries(t *testing.T) {
	tests := []struct {
		score     float64
		wantColor string
	}{
		// >= 80 → green
		{100, "#4c1"},
		{80.1, "#4c1"},
		{80.0, "#4c1"},
		{79.9, "#97ca00"},
		// >= 60 → yellow-green
		{60.0, "#97ca00"},
		{59.9, "#dfb317"},
		// >= 40 → yellow
		{40.0, "#dfb317"},
		{39.9, "#fe7d37"},
		// >= 20 → orange
		{20.0, "#fe7d37"},
		{19.9, "#e05d44"},
		// < 20 → red
		{10.0, "#e05d44"},
		{1.0, "#e05d44"},
		{0, "#e05d44"},
	}

	for _, tt := range tests {
		out, err := badge.GenerateJSON(tt.score)
		if err != nil {
			t.Fatalf("GenerateJSON(%.1f): %v", tt.score, err)
		}
		var fields map[string]interface{}
		json.Unmarshal([]byte(out), &fields)
		if fields["color"] != tt.wantColor {
			t.Errorf("score %.1f: want color %q, got %q", tt.score, tt.wantColor, fields["color"])
		}
	}
}

// TestBadge_GenerateJSON_NegativeScore verifies that negative scores receive
// the red color (fall into the default < 20 bucket) and the message shows
// the negative percentage.
func TestBadge_GenerateJSON_NegativeScore(t *testing.T) {
	out, err := badge.GenerateJSON(-5.0)
	if err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}

	var fields map[string]interface{}
	json.Unmarshal([]byte(out), &fields)

	if fields["color"] != "#e05d44" {
		t.Errorf("negative score: want color #e05d44 (red), got %q", fields["color"])
	}
	if fields["message"] != "-5.0%" {
		t.Errorf("negative score message: want '-5.0%%', got %q", fields["message"])
	}
}

// TestBadge_GenerateJSON_VeryLargeScore verifies that scores above 100 still
// produce valid JSON and get the green color.
func TestBadge_GenerateJSON_VeryLargeScore(t *testing.T) {
	out, err := badge.GenerateJSON(999.9)
	if err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}

	var fields map[string]interface{}
	json.Unmarshal([]byte(out), &fields)

	if fields["color"] != "#4c1" {
		t.Errorf("very large score: want green, got %q", fields["color"])
	}
	if fields["message"] != "999.9%" {
		t.Errorf("very large score message: want '999.9%%', got %q", fields["message"])
	}
}

// ============================================================================
// BADGE — SVG GENERATION
// ============================================================================

// TestBadge_GenerateSVG_ContainsScore verifies the SVG output embeds the
// formatted score string.
func TestBadge_GenerateSVG_ContainsScore(t *testing.T) {
	svg := badge.GenerateSVG(72.3)

	if !strings.Contains(svg, "72.3%") {
		t.Errorf("SVG does not contain the score '72.3%%':\n%s", svg)
	}
}

// TestBadge_GenerateSVG_ContainsColor verifies the correct color hex is
// embedded in the SVG path fill attribute.
func TestBadge_GenerateSVG_ContainsColor(t *testing.T) {
	tests := []struct {
		score     float64
		wantColor string
	}{
		{85.0, "#4c1"},
		{65.0, "#97ca00"},
		{45.0, "#dfb317"},
		{25.0, "#fe7d37"},
		{5.0, "#e05d44"},
	}

	for _, tt := range tests {
		svg := badge.GenerateSVG(tt.score)
		colorAttr := `fill="` + tt.wantColor + `"`
		if !strings.Contains(svg, colorAttr) {
			t.Errorf("score %.1f: SVG missing color %s", tt.score, tt.wantColor)
		}
	}
}

// TestBadge_GenerateSVG_ContainsLabel verifies the SVG contains the
// "mutation" label text.
func TestBadge_GenerateSVG_ContainsLabel(t *testing.T) {
	svg := badge.GenerateSVG(50.0)

	if !strings.Contains(svg, ">mutation<") {
		t.Error("SVG does not contain the 'mutation' label")
	}
}

// TestBadge_GenerateSVG_ValidXML verifies the SVG output is well-formed XML.
func TestBadge_GenerateSVG_ValidXML(t *testing.T) {
	svg := badge.GenerateSVG(88.0)

	// xml.Unmarshal on a string needs a reader
	decoder := xml.NewDecoder(strings.NewReader(svg))
	for {
		_, err := decoder.Token()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("SVG is not valid XML: %v\n%s", err, svg)
		}
	}
}

// TestBadge_GenerateSVG_ColorBoundaries verifies SVG color at each threshold
// boundary, parallel to the JSON color test.
func TestBadge_GenerateSVG_ColorBoundaries(t *testing.T) {
	tests := []struct {
		score     float64
		wantColor string
	}{
		{100, "#4c1"},
		{80.0, "#4c1"},
		{79.9, "#97ca00"},
		{60.0, "#97ca00"},
		{59.9, "#dfb317"},
		{40.0, "#dfb317"},
		{39.9, "#fe7d37"},
		{20.0, "#fe7d37"},
		{19.9, "#e05d44"},
		{0, "#e05d44"},
	}

	for _, tt := range tests {
		svg := badge.GenerateSVG(tt.score)
		if !strings.Contains(svg, `fill="`+tt.wantColor+`"`) {
			t.Errorf("score %.1f: SVG missing color %s", tt.score, tt.wantColor)
		}
	}
}

// TestBadge_GenerateSVG_SchemaRoot verifies the SVG has the correct root
// element with proper namespace.
func TestBadge_GenerateSVG_SchemaRoot(t *testing.T) {
	svg := badge.GenerateSVG(50.0)

	if !strings.HasPrefix(svg, `<svg xmlns="http://www.w3.org/2000/svg"`) {
		t.Errorf("SVG missing proper root element:\n%s", svg)
	}
}

// TestBadge_GenerateSVG_HardcodedWidth documents that the SVG width is
// currently hardcoded at 120 regardless of message length. This test will
// fail if the width ever becomes dynamic, alerting us to review the SVG
// layout for long score messages.
func TestBadge_GenerateSVG_HardcodedWidth(t *testing.T) {
	svg := badge.GenerateSVG(99.9)

	if !strings.Contains(svg, `width="120"`) {
		t.Error("SVG width changed from 120 — review SVG layout for long messages")
	}

	// A very long score (999.9%) should still produce valid SVG within
	// the hardcoded width.
	svgLong := badge.GenerateSVG(999.9)
	if !strings.Contains(svgLong, `width="120"`) {
		t.Error("SVG width varies by score length — may cause layout issues")
	}
	if !strings.Contains(svgLong, "999.9%") {
		t.Error("long score message missing from SVG")
	}
}

// ============================================================================
// BADGE — ROUND-TRIP CONSISTENCY
// ============================================================================

// TestBadge_JSONandSVG_SameColor verifies that JSON and SVG output the same
// color for the same score.
func TestBadge_JSONandSVG_SameColor(t *testing.T) {
	scores := []float64{0, 19.9, 20.0, 39.9, 40.0, 59.9, 60.0, 79.9, 80.0, 100}

	for _, score := range scores {
		jsonOut, err := badge.GenerateJSON(score)
		if err != nil {
			t.Fatalf("GenerateJSON(%.1f): %v", score, err)
		}
		var fields map[string]interface{}
		json.Unmarshal([]byte(jsonOut), &fields)
		jsonColor := fields["color"].(string)

		svgOut := badge.GenerateSVG(score)

		if !strings.Contains(svgOut, `fill="`+jsonColor+`"`) {
			t.Errorf("score %.1f: JSON color %s not found in SVG", score, jsonColor)
		}
	}
}

// TestBadge_JSONandSVG_SameMessage verifies that JSON and SVG show the
// same formatted score message.
func TestBadge_JSONandSVG_SameMessage(t *testing.T) {
	scores := []float64{0, 25.5, 50.0, 75.2, 99.9, 100}

	for _, score := range scores {
		jsonOut, err := badge.GenerateJSON(score)
		if err != nil {
			t.Fatalf("GenerateJSON(%.1f): %v", score, err)
		}
		var fields map[string]interface{}
		json.Unmarshal([]byte(jsonOut), &fields)
		jsonMsg := fields["message"].(string)

		svgOut := badge.GenerateSVG(score)

		if !strings.Contains(svgOut, jsonMsg) {
			t.Errorf("score %.1f: JSON message %q not found in SVG", score, jsonMsg)
		}
	}
}

// ============================================================================
// BADGE — FILE OUTPUT (generateBadge integration)
// ============================================================================

// TestBadge_GenerateBadge_WritesJSONFile verifies that calling the full
// generateBadge path (via the badge package) with a score produces a
// mutation-badge.json file with the correct content.
func TestBadge_GenerateBadge_WritesJSONFile(t *testing.T) {
	dir := t.TempDir()
	score := 78.5

	// Simulate what generateBadge does: call GenerateJSON and write to file
	out, err := badge.GenerateJSON(score)
	if err != nil {
		t.Fatalf("GenerateJSON: %v", err)
	}

	outputPath := filepath.Join(dir, "mutation-badge.json")
	if err := os.WriteFile(outputPath, []byte(out), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(outputPath); err != nil {
		t.Errorf("badge file not created: %v", err)
	}

	// Verify content is valid JSON with correct score
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var fields map[string]interface{}
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatalf("badge file is not valid JSON: %v", err)
	}
	if fields["message"] != "78.5%" {
		t.Errorf("message: want '78.5%%', got %q", fields["message"])
	}
}

// TestBadge_GenerateBadge_WritesSVGFile verifies the full SVG badge file
// output path.
func TestBadge_GenerateBadge_WritesSVGFile(t *testing.T) {
	dir := t.TempDir()
	score := 92.1

	svg := badge.GenerateSVG(score)

	outputPath := filepath.Join(dir, "mutation-badge.svg")
	if err := os.WriteFile(outputPath, []byte(svg), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := os.Stat(outputPath); err != nil {
		t.Errorf("SVG badge file not created: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if !strings.Contains(string(data), "92.1%") {
		t.Error("SVG badge missing score")
	}
	if !strings.Contains(string(data), "#4c1") {
		t.Error("SVG badge missing green color for 92.1%")
	}
}
