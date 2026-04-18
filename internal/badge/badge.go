package badge

import (
	"encoding/json"
	"fmt"
)

// ShieldsIO represents the JSON format for shields.io endpoint badges
type ShieldsIO struct {
	SchemaVersion int    `json:"schemaVersion"`
	Label         string `json:"label"`
	Message       string `json:"message"`
	Color         string `json:"color"`
}

// GenerateJSON creates shields.io JSON format badge
func GenerateJSON(score float64) (string, error) {
	badge := ShieldsIO{
		SchemaVersion: 1,
		Label:         "mutation",
		Message:       fmt.Sprintf("%.1f%%", score),
		Color:         getColor(score),
	}
	data, err := json.MarshalIndent(badge, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// GenerateSVG creates shields.io SVG badge
func GenerateSVG(score float64) string {
	color := getColor(score)
	message := fmt.Sprintf("%.1f%%", score)
	
	// Use shields.io static badge format
	return fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" width="120" height="20">
  <linearGradient id="b" x2="0" y2="100%%">
    <stop offset="0" stop-color="#bbb" stop-opacity=".1"/>
    <stop offset="1" stop-opacity=".1"/>
  </linearGradient>
  <mask id="a">
    <rect width="120" height="20" rx="3" fill="#fff"/>
  </mask>
  <g mask="url(#a)">
    <path fill="#555" d="M0 0h63v20H0z"/>
    <path fill="%s" d="M63 0h57v20H63z"/>
    <path fill="url(#b)" d="M0 0h120v20H0z"/>
  </g>
  <g fill="#fff" text-anchor="middle" font-family="DejaVu Sans,Verdana,Geneva,sans-serif" font-size="11">
    <text x="31.5" y="15" fill="#010101" fill-opacity=".3">mutation</text>
    <text x="31.5" y="14">mutation</text>
    <text x="90.5" y="15" fill="#010101" fill-opacity=".3">%s</text>
    <text x="90.5" y="14">%s</text>
  </g>
</svg>`, color, message, message)
}

// getColor returns the badge color based on mutation score
func getColor(score float64) string {
	switch {
	case score >= 80:
		return "#4c1" // bright green
	case score >= 60:
		return "#97ca00" // yellow-green
	case score >= 40:
		return "#dfb317" // yellow
	case score >= 20:
		return "#fe7d37" // orange
	default:
		return "#e05d44" // red
	}
}
