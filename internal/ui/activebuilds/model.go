package activebuilds

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/marti-batuhankutluay/jenkins-cli/internal/jenkins"
	"github.com/marti-batuhankutluay/jenkins-cli/internal/ui/styles"
)

// OpenMsg is emitted by other screens to navigate here
type OpenMsg struct{}
type BackMsg struct{}

type OpenLogMsg struct {
	JobPath     string
	BuildNumber int
	JobName     string
	EnvName     string
}

type loadedMsg struct {
	builds  []jenkins.RunningBuild
	deploys []jenkins.RunningBuild
}

type errMsg struct{ err error }
type tickMsg time.Time

type Model struct {
	client  *jenkins.Client
	builds  []jenkins.RunningBuild
	deploys []jenkins.RunningBuild
	// flat list for cursor navigation (builds first, then deploys)
	all    []jenkins.RunningBuild
	cursor int

	loading bool
	err     string
	spinner spinner.Model
	width   int
	height  int
}

func New(client *jenkins.Client) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = styles.SpinnerStyle
	return Model{client: client, loading: true, spinner: s}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.load(), m.tick())
}

func (m Model) load() tea.Cmd {
	return func() tea.Msg {
		all, err := m.client.GetRunningBuilds()
		if err != nil {
			return errMsg{err: err}
		}
		var builds, deploys []jenkins.RunningBuild
		for _, b := range all {
			if isDeploy(b.JobName) {
				deploys = append(deploys, b)
			} else {
				builds = append(builds, b)
			}
		}
		return loadedMsg{builds: builds, deploys: deploys}
	}
}

func isDeploy(jobName string) bool {
	lower := strings.ToLower(jobName)
	return strings.Contains(lower, "deploy")
}

func (m Model) tick() tea.Cmd {
	return tea.Tick(1*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}

	case loadedMsg:
		m.loading = false
		m.builds = msg.builds
		m.deploys = msg.deploys
		m.all = append(msg.builds, msg.deploys...)
		if m.cursor >= len(m.all) {
			m.cursor = max(0, len(m.all)-1)
		}
		return m, nil

	case errMsg:
		m.loading = false
		m.err = msg.err.Error()
		return m, nil

	case tickMsg:
		// Silent background refresh — no spinner, don't reset loading state
		return m, tea.Batch(m.tick(), m.silentLoad())

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "esc", "backspace":
			return m, func() tea.Msg { return BackMsg{} }
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.all)-1 {
				m.cursor++
			}
		case "enter", "l":
			if len(m.all) > 0 {
				b := m.all[m.cursor]
				return m, func() tea.Msg {
					return OpenLogMsg{
						JobPath:     b.JobPath,
						BuildNumber: b.Build.Number,
						JobName:     b.JobName,
						EnvName:     b.EnvName,
					}
				}
			}
		}
	}
	return m, nil
}

// silentLoad refreshes data without showing the loading spinner
func (m Model) silentLoad() tea.Cmd {
	return func() tea.Msg {
		all, err := m.client.GetRunningBuilds()
		if err != nil {
			return nil // silently ignore on background refresh
		}
		var builds, deploys []jenkins.RunningBuild
		for _, b := range all {
			if isDeploy(b.JobName) {
				deploys = append(deploys, b)
			} else {
				builds = append(builds, b)
			}
		}
		return loadedMsg{builds: builds, deploys: deploys}
	}
}

func (m Model) View() string {
	if m.height == 0 {
		return ""
	}
	header := styles.HeaderStyle.Width(m.width).Render(" jenkins-cli  Active Builds")
	footer := m.footerView()

	if m.loading {
		return header + "\n" + m.pad(fmt.Sprintf("  %s Loading...", m.spinner.View())) + footer
	}
	if m.err != "" {
		body := styles.ErrorStyle.PaddingLeft(2).Render("✗ Error: "+m.err) + "\n" +
			styles.MutedStyle.PaddingLeft(2).Render("Esc to go back")
		return header + "\n" + m.pad(body) + footer
	}

	var rows []string
	rows = append(rows, "")

	if len(m.builds) == 0 && len(m.deploys) == 0 {
		rows = append(rows, styles.SuccessStyle.PaddingLeft(2).Render("✓ No active builds or deploys"))
		return header + "\n" + m.pad(strings.Join(rows, "\n")) + footer
	}

	// cursor offset so we know which flat index maps to builds vs deploys
	buildOffset := 0
	deployOffset := len(m.builds)

	rows = append(rows, m.sectionHeader("BUILDS", len(m.builds)))
	rows = append(rows, m.colHeader())
	rows = append(rows, styles.MutedStyle.PaddingLeft(2).Render(strings.Repeat("─", max(0, m.width-4))))
	if len(m.builds) == 0 {
		rows = append(rows, styles.MutedStyle.PaddingLeft(4).Render("No active builds"))
	}
	for i, b := range m.builds {
		flatIdx := buildOffset + i
		rows = append(rows, m.renderRow(b, flatIdx))
	}

	rows = append(rows, "")
	rows = append(rows, m.sectionHeader("DEPLOYS", len(m.deploys)))
	rows = append(rows, m.colHeader())
	rows = append(rows, styles.MutedStyle.PaddingLeft(2).Render(strings.Repeat("─", max(0, m.width-4))))
	if len(m.deploys) == 0 {
		rows = append(rows, styles.MutedStyle.PaddingLeft(4).Render("No active deploys"))
	}
	for i, b := range m.deploys {
		flatIdx := deployOffset + i
		rows = append(rows, m.renderRow(b, flatIdx))
	}

	return header + "\n" + m.pad(strings.Join(rows, "\n")) + footer
}

func (m Model) sectionHeader(title string, count int) string {
	label := fmt.Sprintf("  ● %s (%d)", title, count)
	return lipgloss.NewStyle().Bold(true).Foreground(styles.ColorSecondary).Render(label)
}

func (m Model) colHeader() string {
	return lipgloss.NewStyle().PaddingLeft(4).Foreground(styles.ColorMuted).
		Render(fmt.Sprintf("%-20s  %-28s  %-8s  %s", "ENV", "SERVICE", "BUILD #", "ELAPSED"))
}

func (m Model) renderRow(b jenkins.RunningBuild, flatIdx int) string {
	envName := b.EnvName
	if len(envName) > 20 {
		envName = envName[:18] + ".."
	}
	jobName := b.JobName
	if len(jobName) > 28 {
		jobName = jobName[:26] + ".."
	}
	elapsed := jenkins.BuildElapsed(b.Build.Timestamp)
	row := fmt.Sprintf("%s  %-20s  %-28s  %-8s  %s",
		styles.RunningStyle.Render("⟳"),
		envName, jobName,
		fmt.Sprintf("#%d", b.Build.Number),
		styles.RunningStyle.Render(elapsed),
	)
	if flatIdx == m.cursor {
		return styles.SelectedItemStyle.Width(max(0, m.width-2)).Render(row)
	}
	return styles.ItemStyle.PaddingLeft(2).Render(row)
}

func (m Model) pad(body string) string {
	bodyHeight := m.height - 2
	if bodyHeight < 0 {
		bodyHeight = 0
	}
	lines := strings.Count(body, "\n")
	if pad := (bodyHeight - 1) - lines; pad > 0 {
		body += strings.Repeat("\n", pad)
	}
	return body + "\n"
}

func (m Model) footerView() string {
	keys := []struct{ key, desc string }{
		{"↑↓", "navigate"},
		{"Enter", "open log"},
		{"Esc", "back"},
		{"q", "quit"},
	}
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, styles.HelpKeyStyle.Render(k.key)+" "+styles.HelpDescStyle.Render(k.desc))
	}
	return styles.FooterStyle.Width(m.width).Render(strings.Join(parts, "  "))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
