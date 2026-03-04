package joblist

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/marti-batuhankutluay/jenkins-cli/internal/favorites"
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
	Job      jenkins.Job
	JobPath  string
	IsFolder bool
}

type BackMsg struct{}

type Model struct {
	client     *jenkins.Client
	folderPath string
	folderName string
	envName    string
	jobs       []jenkins.Job
	filtered   []jenkins.Job
	favs       *favorites.Favorites
	cursor     int
	filter     string
	loading    bool
	err        string
	spinner    spinner.Model
	width      int
	height     int
	showHelp   bool
	notif      string
	notifTick  int
}

func New(client *jenkins.Client, folderPath, folderName string) Model {
	return NewWithEnv(client, folderPath, folderName, folderName)
}

func NewWithEnv(client *jenkins.Client, folderPath, folderName, envName string) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = styles.SpinnerStyle

	favs, _ := favorites.Load()
	if favs == nil {
		favs = &favorites.Favorites{}
	}

	return Model{
		client:     client,
		folderPath: folderPath,
		folderName: folderName,
		envName:    envName,
		favs:       favs,
		loading:    true,
		spinner:    s,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.load())
}

func (m Model) load() tea.Cmd {
	return func() tea.Msg {
		jobs, err := m.client.GetJobsInFolder(m.folderPath)
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

	case favorites.FavToggledMsg:
		if msg.Added {
			m.notif = "★ Added to favorites: " + msg.Name
		} else {
			m.notif = "☆ Removed from favorites: " + msg.Name
		}
		m.notifTick = 4
		if updated, err := favorites.Load(); err == nil && updated != nil {
			m.favs = updated
		}
		return m, nil

	case tea.KeyMsg:
		if m.loading {
			return m, nil
		}

		// --- Filter mode: intercept all printable keys ---
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
				// confirm selection with current filter
				if len(m.filtered) > 0 {
					job := m.filtered[m.cursor]
					jobPath := m.folderPath + "/" + job.Name
					isFolder := jenkins.IsFolder(job)
					return m, func() tea.Msg {
						return SelectedMsg{Job: job, JobPath: jobPath, IsFolder: isFolder}
					}
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
		case "esc", "backspace":
			return m, func() tea.Msg { return BackMsg{} }
		case "w":
			return m, func() tea.Msg { return activebuilds.OpenMsg{} }
		case "r":
			m.loading = true
			m.err = ""
			m.client.InvalidateCache(m.folderPath)
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
				job := m.filtered[m.cursor]
				jobPath := m.folderPath + "/" + job.Name
				isFolder := jenkins.IsFolder(job)
				return m, func() tea.Msg {
					return SelectedMsg{Job: job, JobPath: jobPath, IsFolder: isFolder}
				}
			}
		case "f":
			if len(m.filtered) > 0 {
				job := m.filtered[m.cursor]
				jobPath := m.folderPath + "/" + job.Name
				jobName := job.Name
				envName := m.envName
				return m, func() tea.Msg {
					return favorites.ToggleFavoriteMsg{Fav: favorites.Favorite{
						Name:    jobName,
						JobPath: jobPath,
						EnvName: envName,
					}}
				}
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

// FolderName returns the display name of the folder
func (m Model) FolderName() string {
	return m.folderName
}

// EnvName returns the root environment name
func (m Model) EnvName() string {
	return m.envName
}

func (m Model) View() string {
	if m.height == 0 {
		return ""
	}
	header := styles.HeaderStyle.Width(m.width).Render(
		fmt.Sprintf(" jenkins-cli  %s  Services", m.folderName),
	)
	footer := m.footerView()

	if m.loading {
		body := fmt.Sprintf("  %s Loading services...", m.spinner.View())
		return header + "\n" + m.pad(body) + footer
	}
	if m.err != "" {
		body := styles.ErrorStyle.PaddingLeft(2).Render("✗ Error: "+m.err) + "\n" +
			styles.MutedStyle.PaddingLeft(2).Render("Press r to retry  •  Esc to go back")
		return header + "\n" + m.pad(body) + footer
	}
	if m.showHelp {
		return header + "\n" + m.pad(m.helpView()) + footer
	}

	var rows []string

	if m.notif != "" {
		rows = append(rows, styles.SuccessStyle.PaddingLeft(2).Render(m.notif))
	}

	if m.filter != "" {
		rows = append(rows,
			styles.FilterStyle.PaddingLeft(2).Render("Filter: ")+
				styles.InputStyle.Render(strings.TrimPrefix(m.filter, "/")))
	}

	// Column header + separator = 2 fixed rows
	colHeader := lipgloss.NewStyle().PaddingLeft(2).Foreground(styles.ColorMuted).
		Render(fmt.Sprintf("%-3s%-2s  %-36s  %-8s  %-20s  %s", " ", " ", "NAME", "BUILD #", "TRIGGERED BY", "STATUS"))
	rows = append(rows, colHeader)
	rows = append(rows, styles.MutedStyle.PaddingLeft(2).Render(strings.Repeat("─", max(0, m.width-4))))

	// Available rows for list items: total height - header(1) - footer(1) - rows so far
	listHeight := m.height - 2 - len(rows)
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
		icon := styles.StatusIcon(job.Color)
		jobPath := m.folderPath + "/" + job.Name
		starIcon := "  "
		if m.favs.Has(jobPath) {
			starIcon = lipgloss.NewStyle().Foreground(styles.ColorText).Render("★ ")
		}
		name := job.Name
		if jenkins.IsFolder(job) {
			name = "▶ " + name
		}
		nameDisplay := name
		if len(name) > 38 {
			nameDisplay = name[:35] + "..."
		}

		buildNum := styles.MutedStyle.Render("-")
		triggeredBy := styles.MutedStyle.Render("-")
		statusStr := styles.MutedStyle.Render("-")

		if job.LastBuild != nil {
			buildNum = styles.MutedStyle.Render(fmt.Sprintf("#%d", job.LastBuild.Number))
			if by := job.LastBuild.TriggeredBy(); by != "" {
				// truncate long names
				if len(by) > 20 {
					by = by[:18] + ".."
				}
				triggeredBy = styles.SubtitleStyle.Render(by)
			}
		}
		if job.Color != "" && job.Color != "notbuilt" {
			status := jenkins.ColorToStatus(job.Color)
			if jenkins.IsRunning(job.Color) {
				statusStr = styles.RunningStyle.Render(status)
				buildNum = styles.RunningStyle.Render(fmt.Sprintf("#%d", job.LastBuild.Number))
			} else {
				switch job.Color {
				case "blue":
					statusStr = styles.SuccessStyle.Render(status)
				case "red":
					statusStr = styles.ErrorStyle.Render(status)
				case "yellow":
					statusStr = styles.WarningStyle.Render(status)
				default:
					statusStr = styles.MutedStyle.Render(status)
				}
			}
		}

		row := fmt.Sprintf("%s%s  %-36s  %-8s  %-20s  %s", icon, starIcon, nameDisplay, buildNum, triggeredBy, statusStr)
		if i == m.cursor {
			rows = append(rows, styles.SelectedItemStyle.Width(max(0, m.width-2)).Render(row))
		} else {
			rows = append(rows, styles.ItemStyle.Width(max(0, m.width-2)).Render(row))
		}
		shown++
	}

	if len(m.filtered) == 0 {
		rows = append(rows, styles.MutedStyle.PaddingLeft(2).Render("No services found"))
	}

	return header + "\n" + m.pad(strings.Join(rows, "\n")) + footer
}

// pad fills remaining height so footer sticks to the bottom.
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
		{"f", "favorite"},
		{"/", "filter"},
		{"w", "active builds"},
		{"r", "refresh"},
		{"Esc", "back"},
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
			fmt.Sprintf("%s      %s", styles.HelpKeyStyle.Render("Enter"), styles.HelpDescStyle.Render("Open job/folder")),
		fmt.Sprintf("%s        %s", styles.HelpKeyStyle.Render("f"), styles.HelpDescStyle.Render("Toggle favorite")),
		fmt.Sprintf("%s        %s", styles.HelpKeyStyle.Render("/"), styles.HelpDescStyle.Render("Filter services")),
		fmt.Sprintf("%s        %s", styles.HelpKeyStyle.Render("r"), styles.HelpDescStyle.Render("Refresh")),
			fmt.Sprintf("%s      %s", styles.HelpKeyStyle.Render("Esc"), styles.HelpDescStyle.Render("Go back")),
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
