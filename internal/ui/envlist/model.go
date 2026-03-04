package envlist

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
	Job jenkins.Job
}

// FavoriteSelectedMsg is emitted when the user selects a favorite directly.
type FavoriteSelectedMsg struct {
	Fav favorites.Favorite
}

// section tracks which section the cursor is in.
type section int

const (
	sectionFavorites section = iota
	sectionEnvs
)

type Model struct {
	client    *jenkins.Client
	jobs      []jenkins.Job
	filtered  []jenkins.Job
	favs      *favorites.Favorites
	cursor    int
	sec       section
	filter    string
	loading   bool
	err       string
	spinner   spinner.Model
	width     int
	height    int
	showHelp  bool
	notif     string
	notifTick int
}

func New(client *jenkins.Client) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = styles.SpinnerStyle

	favs, _ := favorites.Load()
	if favs == nil {
		favs = &favorites.Favorites{}
	}

	return Model{
		client:  client,
		loading: true,
		spinner: s,
		favs:    favs,
		sec:     sectionEnvs,
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
		m.clampCursor()
		return m, nil

	case ErrMsg:
		m.loading = false
		m.err = msg.Err.Error()
		return m, nil

	case favorites.ToggleFavoriteMsg:
		// Reload from disk since app.go already did the toggle
		if updated, err := favorites.Load(); err == nil && updated != nil {
			m.favs = updated
		}
		m.clampCursor()
		return m, nil

	case favorites.FavToggledMsg:
		if msg.Added {
			m.notif = "★ Added to favorites: " + msg.Name
		} else {
			m.notif = "☆ Removed from favorites: " + msg.Name
		}
		m.notifTick = 4
		return m, nil

	case tea.KeyMsg:
		if m.loading {
			return m, nil
		}

		if m.filter != "" {
			return m.handleFilterKey(msg)
		}
		return m.handleNormalKey(msg)
	}

	return m, nil
}

func (m Model) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		m.clampCursor()
	case "enter":
		if m.sec == sectionFavorites && len(m.favs.Items) > 0 {
			fav := m.favs.Items[m.cursor]
			return m, func() tea.Msg { return FavoriteSelectedMsg{Fav: fav} }
		}
		if m.sec == sectionEnvs && len(m.filtered) > 0 {
			idx := m.envCursor()
			return m, func() tea.Msg { return SelectedMsg{Job: m.filtered[idx]} }
		}
	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "ctrl+c":
		return m, tea.Quit
	default:
		m.filter += msg.String()
		m.applyFilter()
		m.cursor = 0
		m.sec = sectionEnvs
	}
	return m, nil
}

func (m Model) handleNormalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "enter":
		if m.sec == sectionFavorites && len(m.favs.Items) > 0 {
			fav := m.favs.Items[m.cursor]
			return m, func() tea.Msg { return FavoriteSelectedMsg{Fav: fav} }
		}
		if m.sec == sectionEnvs && len(m.filtered) > 0 {
			idx := m.envCursor()
			return m, func() tea.Msg { return SelectedMsg{Job: m.filtered[idx]} }
		}
	case "f":
		if m.sec == sectionFavorites && len(m.favs.Items) > 0 {
			fav := m.favs.Items[m.cursor]
			return m, func() tea.Msg {
				return favorites.ToggleFavoriteMsg{Fav: fav}
			}
		}
	case "/":
		m.filter = "/"
		m.sec = sectionEnvs
		m.cursor = 0
	}
	return m, nil
}

// moveCursor moves up/down across both sections.
func (m *Model) moveCursor(delta int) {
	favCount := len(m.favs.Items)
	envCount := len(m.filtered)

	if m.sec == sectionFavorites {
		newPos := m.cursor + delta
		if newPos < 0 {
			return
		}
		if newPos >= favCount {
			if envCount > 0 {
				m.sec = sectionEnvs
				m.cursor = 0
			}
			return
		}
		m.cursor = newPos
	} else {
		idx := m.envCursor()
		newIdx := idx + delta
		if newIdx < 0 {
			if favCount > 0 {
				m.sec = sectionFavorites
				m.cursor = favCount - 1
			}
			return
		}
		if newIdx >= envCount {
			return
		}
		m.cursor = newIdx
	}
}

// envCursor returns the cursor index within the envs section.
func (m *Model) envCursor() int {
	if m.sec == sectionEnvs {
		return m.cursor
	}
	return 0
}

func (m *Model) clampCursor() {
	favCount := len(m.favs.Items)
	envCount := len(m.filtered)

	if m.sec == sectionFavorites {
		if favCount == 0 {
			m.sec = sectionEnvs
			m.cursor = 0
		} else if m.cursor >= favCount {
			m.cursor = favCount - 1
		}
	} else {
		if m.cursor >= envCount {
			m.cursor = max(0, envCount-1)
		}
	}
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

	if m.notif != "" {
		rows = append(rows, styles.SuccessStyle.PaddingLeft(2).Render(m.notif))
	}

	if m.filter != "" {
		rows = append(rows,
			styles.FilterStyle.PaddingLeft(2).Render("Filter: ")+
				styles.InputStyle.Render(strings.TrimPrefix(m.filter, "/")))
	}

	// Favorites section
	if len(m.favs.Items) > 0 {
		rows = append(rows, "")
		rows = append(rows, styles.MutedStyle.PaddingLeft(2).Render("★ Favorites"))
		rows = append(rows, styles.MutedStyle.PaddingLeft(2).Render(strings.Repeat("─", max(0, m.width-4))))
		for i, fav := range m.favs.Items {
			line := "★ " + fav.Name
			if fav.EnvName != "" && fav.EnvName != fav.Name {
				line += styles.MutedStyle.Render("  "+fav.EnvName)
			}
			selected := m.sec == sectionFavorites && m.cursor == i
			if selected {
				rows = append(rows, styles.SelectedItemStyle.Width(max(0, m.width-2)).Render(line))
			} else {
				rows = append(rows, styles.ItemStyle.Width(max(0, m.width-2)).Render(line))
			}
		}
		rows = append(rows, "")
		rows = append(rows, styles.MutedStyle.PaddingLeft(2).Render("Environments"))
		rows = append(rows, styles.MutedStyle.PaddingLeft(2).Render(strings.Repeat("─", max(0, m.width-4))))
	} else {
		rows = append(rows, "")
	}

	// Environments section
	fixedRows := len(rows)
	listHeight := m.height - 2 - fixedRows
	if listHeight < 1 {
		listHeight = 1
	}

	envIdx := 0
	if m.sec == sectionEnvs {
		envIdx = m.cursor
	}

	viewStart := 0
	if envIdx >= listHeight {
		viewStart = envIdx - listHeight + 1
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
		selected := m.sec == sectionEnvs && m.cursor == i
		if selected {
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
		{"f", "unfavorite"},
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
			fmt.Sprintf("%s      %s", styles.HelpKeyStyle.Render("Enter"), styles.HelpDescStyle.Render("Select / open favorite")),
			fmt.Sprintf("%s        %s", styles.HelpKeyStyle.Render("f"), styles.HelpDescStyle.Render("Remove favorite (on favorite row)")),
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
