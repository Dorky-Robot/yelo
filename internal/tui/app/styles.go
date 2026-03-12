package app

import "github.com/charmbracelet/lipgloss"

// Color palette — matches tunnels' visual language.
var (
	cyan   = lipgloss.Color("6")  // Primary action / highlight
	green  = lipgloss.Color("2")  // Success / running / STANDARD
	red    = lipgloss.Color("1")  // Danger / DEEP_ARCHIVE
	yellow = lipgloss.Color("3")  // Warning / restoring
	dim    = lipgloss.Color("8")  // Secondary / inactive
	white  = lipgloss.Color("15") // Active input
	blue   = lipgloss.Color("4")  // GLACIER_IR
	orange = lipgloss.Color("208") // GLACIER

	// Selection highlight background
	selectBg = lipgloss.Color("#1e2837")
)

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
	case "STANDARD_IA", "ONEZONE_IA", "INTELLIGENT_TIERING":
		return dim
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
