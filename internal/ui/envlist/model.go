package envlist

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/marti-batuhankutluay/jenkins-cli/internal/jenkins"
	"github.com/marti-batuhankutluay/jenkins-cli/internal/ui/activebuilds"
	"github.com/marti-batuhankutluay/jenkins-cli/internal/ui/styles"
)

type LoadedMsg struct {
	Jobs []jenkins.Job
}

type ErrMsg struct {
	Err error
}

type SelectedMsg struct {
	Job jenkins.Job
}

type Model struct {
	client   *jenkins.Client
	jobs     []jenkins.Job
	filtered []jenkins.Job
	cursor   int
	filter   string
	loading  bool
	err      string
	spinner  spinner.Model
	width    int
	height   int
	showHelp bool
}

func New(client *jenkins.Client) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = styles.SpinnerStyle

	return Model{
		client:  client,
		loading: true,
		spinner: s,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.load())
}

func (m Model) load() tea.Cmd {
	return func() tea.Msg {
		jobs, err := m.client.GetJobs()
		if err != nil {
			return ErrMsg{Err: err}
		}
		return LoadedMsg{Jobs: jobs}
	}
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

	case LoadedMsg:
		m.loading = false
		m.jobs = msg.Jobs
		m.applyFilter()
		return m, nil

	case ErrMsg:
		m.loading = false
		m.err = msg.Err.Error()
		return m, nil

	case tea.KeyMsg:
		if m.loading {
			return m, nil
		}

		// --- Filter mode: all printable keys go to the filter ---
		if m.filter != "" {
			switch msg.String() {
			case "esc":
				m.filter = ""
				m.applyFilter()
			case "backspace":
				if len(m.filter) > 1 {
					m.filter = m.filter[:len(m.filter)-1]
				} else {
					m.filter = ""
				}
				m.applyFilter()
				if m.cursor >= len(m.filtered) {
					m.cursor = max(0, len(m.filtered)-1)
				}
			case "enter":
				if len(m.filtered) > 0 {
					return m, func() tea.Msg { return SelectedMsg{Job: m.filtered[m.cursor]} }
				}
			case "up":
				if m.cursor > 0 {
					m.cursor--
				}
			case "down":
				if m.cursor < len(m.filtered)-1 {
					m.cursor++
				}
			case "ctrl+c":
				return m, tea.Quit
			default:
				m.filter += msg.String()
				m.applyFilter()
				m.cursor = 0
			}
			return m, nil
		}

		// --- Normal mode ---
		switch msg.String() {
		case "?":
			m.showHelp = !m.showHelp
		case "q", "ctrl+c":
			return m, tea.Quit
		case "w":
			return m, func() tea.Msg { return activebuilds.OpenMsg{} }
		case "r":
			m.loading = true
			m.err = ""
			m.client.InvalidateCache("__root__")
			return m, tea.Batch(m.spinner.Tick, m.load())
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
		case "enter":
			if len(m.filtered) > 0 {
				return m, func() tea.Msg { return SelectedMsg{Job: m.filtered[m.cursor]} }
			}
		case "/":
			m.filter = "/"
		}
	}

	return m, nil
}

func (m *Model) applyFilter() {
	if m.filter == "" || m.filter == "/" {
		m.filtered = m.jobs
		return
	}
	query := strings.ToLower(strings.TrimPrefix(m.filter, "/"))
	m.filtered = nil
	for _, j := range m.jobs {
		if strings.Contains(strings.ToLower(j.Name), query) {
			m.filtered = append(m.filtered, j)
		}
	}
}

func (m Model) View() string {
	if m.height == 0 {
		return ""
	}
	header := styles.HeaderStyle.Width(m.width).Render(" jenkins-cli  Environments")
	footer := m.footerView()

	if m.loading {
		body := fmt.Sprintf("  %s Loading environments...", m.spinner.View())
		return header + "\n" + m.pad(body) + footer
	}
	if m.err != "" {
		body := styles.ErrorStyle.PaddingLeft(2).Render("✗ Error: "+m.err) + "\n" +
			styles.MutedStyle.PaddingLeft(2).Render("Press r to retry")
		return header + "\n" + m.pad(body) + footer
	}
	if m.showHelp {
		return header + "\n" + m.pad(m.helpView()) + footer
	}

	var rows []string
	if m.filter != "" {
		rows = append(rows,
			styles.FilterStyle.PaddingLeft(2).Render("Filter: ")+
				styles.InputStyle.Render(strings.TrimPrefix(m.filter, "/")))
	}
	rows = append(rows, "") // blank line below header / filter

	listHeight := m.height - 2 - len(rows) // header + footer accounted, minus rows above
	if listHeight < 1 {
		listHeight = 1
	}

	viewStart := 0
	if m.cursor >= listHeight {
		viewStart = m.cursor - listHeight + 1
	}

	shown := 0
	for i, job := range m.filtered {
		if i < viewStart || shown >= listHeight {
			continue
		}
		icon := "  "
		if jenkins.IsFolder(job) {
			icon = styles.MutedStyle.Render("▶ ")
		}
		line := icon + job.Name
		if i == m.cursor {
			rows = append(rows, styles.SelectedItemStyle.Width(max(0, m.width-2)).Render(line))
		} else {
			rows = append(rows, styles.ItemStyle.Width(max(0, m.width-2)).Render(line))
		}
		shown++
	}

	if len(m.filtered) == 0 {
		rows = append(rows, styles.MutedStyle.PaddingLeft(2).Render("No environments found"))
	}

	return header + "\n" + m.pad(strings.Join(rows, "\n")) + footer
}

// pad adds newlines so that body + footer fills m.height exactly.
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
		{"Enter", "select"},
		{"/", "filter"},
		{"w", "active builds"},
		{"r", "refresh"},
		{"?", "help"},
		{"q", "quit"},
	}
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, styles.HelpKeyStyle.Render(k.key)+" "+styles.HelpDescStyle.Render(k.desc))
	}
	return styles.FooterStyle.Width(m.width).Render(strings.Join(parts, "  "))
}

func (m Model) helpView() string {
	help := styles.BoxStyle.Render(
		lipgloss.JoinVertical(lipgloss.Left,
			styles.TitleStyle.Render("Keyboard Shortcuts"),
			"",
			fmt.Sprintf("%s  %s", styles.HelpKeyStyle.Render("↑/k  ↓/j"), styles.HelpDescStyle.Render("Navigate")),
			fmt.Sprintf("%s      %s", styles.HelpKeyStyle.Render("Enter"), styles.HelpDescStyle.Render("Select environment")),
			fmt.Sprintf("%s        %s", styles.HelpKeyStyle.Render("/"), styles.HelpDescStyle.Render("Filter")),
			fmt.Sprintf("%s        %s", styles.HelpKeyStyle.Render("r"), styles.HelpDescStyle.Render("Refresh")),
			fmt.Sprintf("%s        %s", styles.HelpKeyStyle.Render("?"), styles.HelpDescStyle.Render("Toggle help")),
			fmt.Sprintf("%s        %s", styles.HelpKeyStyle.Render("q"), styles.HelpDescStyle.Render("Quit")),
		),
	)
	return "\n" + lipgloss.NewStyle().PaddingLeft(2).Render(help)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
