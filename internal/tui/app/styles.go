package app

import (
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
)

// Color palette.
var (
	cyan   = lipgloss.Color("6")
	green  = lipgloss.Color("2")
	red    = lipgloss.Color("1")
	yellow = lipgloss.Color("3")
	dim    = lipgloss.Color("8")
	white  = lipgloss.Color("15")
	blue   = lipgloss.Color("4")
	orange = lipgloss.Color("208")

	selectBg = lipgloss.Color("#1e2837")
)

func tableStyles() table.Styles {
	s := table.DefaultStyles()
	s.Header = s.Header.
		Foreground(cyan).
		Bold(true).
		BorderBottom(true).
		BorderBottomForeground(dim)
	s.Selected = s.Selected.
		Background(selectBg).
		Bold(true).
		Foreground(lipgloss.NoColor{})
	s.Cell = s.Cell.
		Foreground(lipgloss.NoColor{})
	return s
}

func storageClassColor(class string) lipgloss.Color {
	switch class {
	case "DEEP_ARCHIVE":
		return red
	case "GLACIER":
		return orange
	case "GLACIER_IR":
		return blue
	case "STANDARD":
		return green
	default:
		return dim
	}
}

func storageClassLabel(class string) string {
	switch class {
	case "DEEP_ARCHIVE":
		return "DEEP"
	case "GLACIER":
		return "GLACIER"
	case "GLACIER_IR":
		return "GLCR_IR"
	case "STANDARD":
		return "STD"
	case "STANDARD_IA":
		return "STD_IA"
	case "ONEZONE_IA":
		return "OZ_IA"
	case "INTELLIGENT_TIERING":
		return "INT_T"
	default:
		return class
	}
}

func restoreLabel(status string) (string, lipgloss.Color) {
	switch status {
	case "in-progress":
		return "restoring", yellow
	case "available":
		return "available", green
	default:
		return "", dim
	}
}
