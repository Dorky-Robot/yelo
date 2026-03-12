package app

import "github.com/charmbracelet/lipgloss"

var (
	// Colors
	colorBorder    = lipgloss.Color("240")
	colorTitle     = lipgloss.Color("39")
	colorSelected  = lipgloss.Color("170")
	colorPrefix    = lipgloss.Color("39")
	colorGlacier   = lipgloss.Color("208")
	colorDeep      = lipgloss.Color("196")
	colorStandard  = lipgloss.Color("40")
	colorGlacierIR = lipgloss.Color("75")
	colorRestoring = lipgloss.Color("226")
	colorAvailable = lipgloss.Color("40")
	colorDim       = lipgloss.Color("242")
	colorHelp      = lipgloss.Color("241")
	colorError     = lipgloss.Color("196")
	colorFlash     = lipgloss.Color("229")

	// Header bar
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorTitle).
			Padding(0, 1)

	// List panel
	listBorderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder)

	// Detail panel
	detailBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorBorder)

	// Items
	selectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorSelected)

	prefixStyle = lipgloss.NewStyle().
			Foreground(colorPrefix)

	dimStyle = lipgloss.NewStyle().
			Foreground(colorDim)

	// Detail labels
	labelStyle = lipgloss.NewStyle().
			Foreground(colorDim).
			Width(10).
			Align(lipgloss.Right)

	// Status bar
	statusBarStyle = lipgloss.NewStyle().
			Foreground(colorHelp).
			Padding(0, 1)

	errorStyle = lipgloss.NewStyle().
			Foreground(colorError).
			Bold(true)

	flashStyle = lipgloss.NewStyle().
			Foreground(colorFlash)
)

// storageClassStyle returns styled text for a storage class.
func storageClassStyle(class string) string {
	switch class {
	case "DEEP_ARCHIVE":
		return lipgloss.NewStyle().Foreground(colorDeep).Render("DEEP")
	case "GLACIER":
		return lipgloss.NewStyle().Foreground(colorGlacier).Render("GLCR")
	case "GLACIER_IR":
		return lipgloss.NewStyle().Foreground(colorGlacierIR).Render("GL_IR")
	case "STANDARD":
		return lipgloss.NewStyle().Foreground(colorStandard).Render("STD")
	case "STANDARD_IA", "ONEZONE_IA":
		return lipgloss.NewStyle().Foreground(colorDim).Render("IA")
	case "INTELLIGENT_TIERING":
		return lipgloss.NewStyle().Foreground(colorDim).Render("INT_T")
	default:
		return dimStyle.Render(class)
	}
}

func restoreStatusStyle(status string) string {
	switch status {
	case "in-progress":
		return lipgloss.NewStyle().Foreground(colorRestoring).Render("restoring...")
	case "available":
		return lipgloss.NewStyle().Foreground(colorAvailable).Render("available")
	default:
		return dimStyle.Render("--")
	}
}
