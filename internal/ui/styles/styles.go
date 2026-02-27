package styles

import "github.com/charmbracelet/lipgloss"

var (
	// Base colors
	ColorPrimary   = lipgloss.Color("#7C3AED") // purple
	ColorSecondary = lipgloss.Color("#06B6D4") // cyan
	ColorSuccess   = lipgloss.Color("#10B981") // green
	ColorError     = lipgloss.Color("#EF4444") // red
	ColorWarning   = lipgloss.Color("#F59E0B") // amber
	ColorMuted     = lipgloss.Color("#6B7280") // gray
	ColorText      = lipgloss.Color("#F9FAFB") // near white
	ColorSubtle    = lipgloss.Color("#9CA3AF") // lighter gray
	ColorRunning   = lipgloss.Color("#3B82F6") // blue

	// Header bar
	HeaderStyle = lipgloss.NewStyle().
			Background(ColorPrimary).
			Foreground(lipgloss.Color("#FFFFFF")).
			Bold(true).
			Padding(0, 2)

	HeaderTitleStyle = lipgloss.NewStyle().
				Background(ColorPrimary).
				Foreground(lipgloss.Color("#FFFFFF")).
				Bold(true)

	HeaderPathStyle = lipgloss.NewStyle().
			Background(ColorPrimary).
			Foreground(lipgloss.Color("#DDD6FE")).
			Padding(0, 1)

	// Footer / help bar
	FooterStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#1F2937")).
			Foreground(ColorSubtle).
			Padding(0, 1)

	HelpKeyStyle = lipgloss.NewStyle().
			Foreground(ColorSecondary).
			Bold(true)

	HelpDescStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	// List items
	ItemStyle = lipgloss.NewStyle().
			Padding(0, 2)

	SelectedItemStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#374151")).
				Foreground(ColorText).
				Bold(true).
				Padding(0, 2)

	// Status icons
	StatusSuccess = lipgloss.NewStyle().Foreground(ColorSuccess).Render("✓")
	StatusFailed  = lipgloss.NewStyle().Foreground(ColorError).Render("✗")
	StatusRunning = lipgloss.NewStyle().Foreground(ColorRunning).Render("⟳")
	StatusUnstable = lipgloss.NewStyle().Foreground(ColorWarning).Render("⚠")
	StatusDisabled = lipgloss.NewStyle().Foreground(ColorMuted).Render("○")
	StatusAborted  = lipgloss.NewStyle().Foreground(ColorMuted).Render("⊘")
	StatusUnknown  = lipgloss.NewStyle().Foreground(ColorMuted).Render("?")

	// Text styles
	TitleStyle = lipgloss.NewStyle().
			Foreground(ColorText).
			Bold(true)

	SubtitleStyle = lipgloss.NewStyle().
			Foreground(ColorSubtle)

	MutedStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	SuccessStyle = lipgloss.NewStyle().
			Foreground(ColorSuccess)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(ColorError)

	WarningStyle = lipgloss.NewStyle().
			Foreground(ColorWarning)

	RunningStyle = lipgloss.NewStyle().
			Foreground(ColorRunning)

	// Warning box
	WarningBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorWarning).
			Padding(1, 2).
			Margin(1, 2)

	// Input styles
	InputLabelStyle = lipgloss.NewStyle().
			Foreground(ColorSecondary).
			Bold(true)

	InputStyle = lipgloss.NewStyle().
			Foreground(ColorText)

	// Spinner
	SpinnerStyle = lipgloss.NewStyle().
			Foreground(ColorSecondary)

	// Border box
	BoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimary).
			Padding(0, 1)

	// Filter input
	FilterStyle = lipgloss.NewStyle().
			Foreground(ColorSecondary).
			Bold(true)

	// Build number
	BuildNumStyle = lipgloss.NewStyle().
			Foreground(ColorSubtle)

	// Dimmed row (non-selected)
	DimStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)
)

// StatusIcon returns the icon for a Jenkins color string
func StatusIcon(color string) string {
	switch {
	case color == "blue":
		return StatusSuccess
	case color == "red":
		return StatusFailed
	case color == "yellow":
		return StatusUnstable
	case color == "grey", color == "disabled", color == "notbuilt":
		return StatusDisabled
	case color == "aborted":
		return StatusAborted
	case len(color) > 6 && color[len(color)-6:] == "_anime":
		return StatusRunning
	default:
		return StatusUnknown
	}
}

// ResultIcon returns icon for a build result string
func ResultIcon(result string, building bool) string {
	if building {
		return StatusRunning
	}
	switch result {
	case "SUCCESS":
		return StatusSuccess
	case "FAILURE":
		return StatusFailed
	case "UNSTABLE":
		return StatusUnstable
	case "ABORTED":
		return StatusAborted
	default:
		return StatusUnknown
	}
}
