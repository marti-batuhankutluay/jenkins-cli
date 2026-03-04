package jobdetail

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/marti-batuhankutluay/jenkins-cli/internal/favorites"
	"github.com/marti-batuhankutluay/jenkins-cli/internal/jenkins"
	"github.com/marti-batuhankutluay/jenkins-cli/internal/ui/styles"
)

// Messages
type LoadedMsg struct {
	Detail *jenkins.JobDetail
}

type ErrMsg struct{ Err error }
type BackMsg struct{}
type TriggerBuildMsg struct{ JobPath string }
type OpenLogMsg struct {
	JobPath     string
	BuildNumber int
}

type BuildTriggeredMsg struct{}
type BuildTriggerErrMsg struct{ Err error }

type runningCheckMsg struct {
	lastBuild *jenkins.Build
	err       error
}

type tickMsg time.Time

type state int

const (
	stateLoading state = iota
	stateNormal
	stateWarning  // "build already running" confirmation
	stateTriggering
	stateErr
)

type Model struct {
	client           *jenkins.Client
	jobPath          string
	jobName          string
	envName          string
	detail           *jenkins.JobDetail
	cursor           int
	st               state
	err              string
	spinner          spinner.Model
	warning          string // warning message for running build
	waitingKey       string // "b" or "d"
	deployBuildValue string // build number/displayName to pass as param on deploy
	width            int
	height           int
	showHelp         bool
	notification     string
	notifyTimer      int
}

func New(client *jenkins.Client, jobPath, jobName, envName string) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = styles.SpinnerStyle

	return Model{
		client:  client,
		jobPath: jobPath,
		jobName: jobName,
		envName: envName,
		st:      stateLoading,
		spinner: s,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.load(), m.tick())
}

func (m Model) load() tea.Cmd {
	return func() tea.Msg {
		detail, err := m.client.GetJobDetail(m.jobPath)
		if err != nil {
			return ErrMsg{Err: err}
		}
		return LoadedMsg{Detail: detail}
	}
}

func (m Model) tick() tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
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
		if m.st == stateLoading || m.st == stateTriggering {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}

	case tickMsg:
		if m.notifyTimer > 0 {
			m.notifyTimer--
			if m.notifyTimer == 0 {
				m.notification = ""
			}
		}
		// Auto-refresh if a build is running
		var cmds []tea.Cmd
		cmds = append(cmds, m.tick())
		if m.st == stateNormal && m.detail != nil && m.detail.LastBuild != nil && m.detail.LastBuild.Building {
			cmds = append(cmds, m.load())
		}
		return m, tea.Batch(cmds...)

	case LoadedMsg:
		m.st = stateNormal
		m.detail = msg.Detail
		return m, nil

	case ErrMsg:
		m.st = stateErr
		m.err = msg.Err.Error()
		return m, nil

	case runningCheckMsg:
		if msg.err != nil {
			m.st = stateNormal
			m.notification = "⚠ Could not check build status: " + msg.err.Error()
			m.notifyTimer = 5
			return m, nil
		}
		if msg.lastBuild != nil && msg.lastBuild.Building {
			m.st = stateWarning
			elapsed := jenkins.BuildElapsed(msg.lastBuild.Timestamp)
			m.warning = fmt.Sprintf("⚠  Build #%d is already running (%s)\nTrigger another? [y/N]",
				msg.lastBuild.Number, elapsed)
		} else {
			return m, m.triggerBuild()
		}
		return m, nil

	case BuildTriggeredMsg:
		m.st = stateNormal
		if m.waitingKey == "d" && m.deployBuildValue != "" {
			m.notification = fmt.Sprintf("✓ Deploy triggered (build %s)", m.deployBuildValue)
		} else {
			m.notification = "✓ Build triggered successfully!"
		}
		m.notifyTimer = 5
		return m, tea.Batch(m.load(), m.spinner.Tick)

	case BuildTriggerErrMsg:
		m.st = stateNormal
		m.notification = "✗ Failed to trigger build: " + msg.Err.Error()
		m.notifyTimer = 5
		return m, nil

	case tea.KeyMsg:
		switch m.st {
		case stateWarning:
			return m.handleWarningKey(msg)
		case stateLoading, stateTriggering:
			return m, nil
		case stateErr:
			switch msg.String() {
			case "r":
				m.st = stateLoading
				return m, tea.Batch(m.spinner.Tick, m.load())
			case "esc", "backspace", "q":
				return m, func() tea.Msg { return BackMsg{} }
			}
		default:
			return m.handleNormalKey(msg)
		}
	}

	return m, nil
}

func (m Model) handleNormalKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "?":
		m.showHelp = !m.showHelp
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc", "backspace":
		return m, func() tea.Msg { return BackMsg{} }
	case "r":
		m.st = stateLoading
		return m, tea.Batch(m.spinner.Tick, m.load())
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.detail != nil && m.cursor < len(m.detail.Builds)-1 {
			m.cursor++
		}
	case "b":
		m.waitingKey = "b"
		m.deployBuildValue = ""
		return m, m.checkRunning()
	case "d":
		m.waitingKey = "d"
		m.deployBuildValue = ""
		if m.detail != nil && m.cursor < len(m.detail.Builds) {
			b := m.detail.Builds[m.cursor]
			// Prefer upstream build number (e.g. 311 from api-service/Build)
			if up := b.UpstreamBuildNumber(); up > 0 {
				m.deployBuildValue = fmt.Sprintf("%d", up)
			} else if b.DisplayName != "" {
				m.deployBuildValue = strings.TrimPrefix(b.DisplayName, "#")
			} else {
				m.deployBuildValue = fmt.Sprintf("%d", b.Number)
			}
		}
		return m, m.checkRunning()
	case "l":
		if m.detail != nil && m.detail.LastBuild != nil {
			num := m.detail.LastBuild.Number
			if m.cursor < len(m.detail.Builds) {
				num = m.detail.Builds[m.cursor].Number
			}
			jobPath := m.jobPath
			return m, func() tea.Msg {
				return OpenLogMsg{JobPath: jobPath, BuildNumber: num}
			}
		}
	case "f":
		jobPath := m.jobPath
		jobName := m.jobName
		envName := m.envName
		return m, func() tea.Msg {
			return favorites.ToggleFavoriteMsg{Fav: favorites.Favorite{
				Name:    jobName,
				JobPath: jobPath,
				EnvName: envName,
			}}
		}
	}
	return m, nil
}

func (m Model) handleWarningKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		m.st = stateTriggering
		m.warning = ""
		return m, tea.Batch(m.spinner.Tick, m.triggerBuild())
	case "n", "N", "esc", "enter":
		m.st = stateNormal
		m.warning = ""
	case "ctrl+c", "q":
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) checkRunning() tea.Cmd {
	return func() tea.Msg {
		build, err := m.client.GetLastBuild(m.jobPath)
		return runningCheckMsg{lastBuild: build, err: err}
	}
}

func (m Model) triggerBuild() tea.Cmd {
	jobPath := m.jobPath
	isDeploy := m.waitingKey == "d"
	deployValue := m.deployBuildValue

	return func() tea.Msg {
		client := m.client

		if isDeploy && deployValue != "" {
			// Fetch parameter definitions to find the right param name.
			// Priority: exact/contains "tag" first, then "build", then default.
			paramName := "BUILD_NUMBER"
			if params, err := client.GetJobParamDefinitions(jobPath); err == nil {
				// First pass: look for "tag" param (e.g. deploy.tag)
				for _, p := range params {
					if strings.EqualFold(p.Name, "tag") || strings.Contains(strings.ToLower(p.Name), "tag") {
						paramName = p.Name
						break
					}
				}
				// Second pass: fallback to "build" param
				if paramName == "BUILD_NUMBER" {
					for _, p := range params {
						if strings.Contains(strings.ToLower(p.Name), "build") {
							paramName = p.Name
							break
						}
					}
				}
			}
			if err := client.TriggerBuildWithParams(jobPath, map[string]string{paramName: deployValue}); err != nil {
				return BuildTriggerErrMsg{Err: err}
			}
		} else {
			if err := client.TriggerBuild(jobPath); err != nil {
				return BuildTriggerErrMsg{Err: err}
			}
		}
		return BuildTriggeredMsg{}
	}
}

func (m Model) View() string {
	if m.height == 0 {
		return ""
	}
	header := styles.HeaderStyle.Width(m.width).Render(
		fmt.Sprintf(" jenkins-cli  %s  %s", m.envName, m.jobName),
	)
	footer := m.footerView()

	switch m.st {
	case stateLoading:
		return header + "\n" + m.pad(fmt.Sprintf("  %s Loading job details...", m.spinner.View())) + footer
	case stateTriggering:
		return header + "\n" + m.pad(fmt.Sprintf("  %s Triggering build...", m.spinner.View())) + footer
	case stateErr:
		body := styles.ErrorStyle.PaddingLeft(2).Render("✗ Error: "+m.err) + "\n" +
			styles.MutedStyle.PaddingLeft(2).Render("Press r to retry  •  Esc to go back")
		return header + "\n" + m.pad(body) + footer
	}

	if m.showHelp {
		return header + "\n" + m.pad(m.helpView()) + footer
	}
	if m.detail == nil {
		return header + "\n" + m.pad("") + footer
	}

	var rows []string

	// Notification
	if m.notification != "" {
		var notifStyle lipgloss.Style
		switch {
		case strings.HasPrefix(m.notification, "✓"):
			notifStyle = styles.SuccessStyle.PaddingLeft(2)
		case strings.HasPrefix(m.notification, "✗"):
			notifStyle = styles.ErrorStyle.PaddingLeft(2)
		default:
			notifStyle = styles.WarningStyle.PaddingLeft(2)
		}
		rows = append(rows, notifStyle.Render(m.notification))
	}

	// Job info
	rows = append(rows, "") // blank line
	icon := styles.StatusIcon(m.detail.Color)
	status := jenkins.ColorToStatus(m.detail.Color)
	statusDisplay := icon + " " + status
	if jenkins.IsRunning(m.detail.Color) {
		statusDisplay = icon + " " + styles.RunningStyle.Render(status)
	}
	rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top,
		styles.TitleStyle.PaddingLeft(2).Render(m.detail.Name),
		styles.MutedStyle.PaddingLeft(2).Render("  "+statusDisplay),
	))
	if m.detail.Description != "" {
		rows = append(rows, styles.SubtitleStyle.PaddingLeft(2).Render(m.detail.Description))
	}
	if m.detail.LastBuild != nil && m.detail.LastBuild.Building {
		elapsed := jenkins.BuildElapsed(m.detail.LastBuild.Timestamp)
		rows = append(rows, styles.WarningStyle.PaddingLeft(2).Render(
			fmt.Sprintf("⟳ Build #%d is running (%s)", m.detail.LastBuild.Number, elapsed),
		))
	}
	rows = append(rows, "") // blank line before table

	// Table header + separator
	colHeader := lipgloss.NewStyle().PaddingLeft(2).Foreground(styles.ColorMuted).
		Render(fmt.Sprintf("%-3s  %-8s  %-10s  %-10s  %-10s  %-18s  %s", " ", "BUILD #", "UPSTREAM", "RESULT", "DURATION", "TRIGGERED BY", "STARTED"))
	rows = append(rows, colHeader)
	rows = append(rows, styles.MutedStyle.PaddingLeft(2).Render(strings.Repeat("─", max(0, m.width-4))))

	// List area: remaining height
	listHeight := m.height - 2 - len(rows)
	if m.st == stateWarning {
		listHeight -= 5 // reserve space for warning box
	}
	if listHeight < 1 {
		listHeight = 1
	}

	viewStart := 0
	if m.cursor >= listHeight {
		viewStart = m.cursor - listHeight + 1
	}

	shown := 0
	for i, build := range m.detail.Builds {
		if i < viewStart || shown >= listHeight {
			continue
		}
		bIcon := styles.ResultIcon(build.Result, build.Building)
		buildNum := fmt.Sprintf("#%d", build.Number)
		upstreamNum := "-"
		if up := build.UpstreamBuildNumber(); up > 0 {
			upstreamNum = fmt.Sprintf("#%d", up)
		}
		result := build.Result
		if build.Building {
			result = "RUNNING"
		}
		if result == "" {
			result = "-"
		}
		by := build.TriggeredBy()
		if by == "" {
			by = "-"
		} else if len(by) > 18 {
			by = by[:16] + ".."
		}
		row := fmt.Sprintf("%s  %-8s  %-10s  %-10s  %-10s  %-18s  %s",
			bIcon, buildNum, upstreamNum, result,
			jenkins.FormatDuration(build.Duration),
			by,
			jenkins.FormatTimestamp(build.Timestamp),
		)
		if i == m.cursor {
			rows = append(rows, styles.SelectedItemStyle.Width(max(0, m.width-2)).Render(row))
		} else {
			rows = append(rows, styles.ItemStyle.Width(max(0, m.width-2)).Render(row))
		}
		shown++
	}

	if len(m.detail.Builds) == 0 {
		rows = append(rows, styles.MutedStyle.PaddingLeft(2).Render("No builds yet"))
	}

	// Warning box (confirmation prompt)
	if m.st == stateWarning {
		rows = append(rows, "")
		rows = append(rows, lipgloss.NewStyle().PaddingLeft(2).Render(
			styles.WarningBoxStyle.Render(m.warning),
		))
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
		{"b", "build"},
		{"d", "deploy"},
		{"l", "log"},
		{"f", "favorite"},
		{"r", "refresh"},
		{"Esc", "back"},
		{"?", "help"},
	}
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, styles.HelpKeyStyle.Render(k.key)+" "+styles.HelpDescStyle.Render(k.desc))
	}
	return styles.FooterStyle.Width(m.width).Render(strings.Join(parts, "  "))
}

// JobName returns the job display name
func (m Model) JobName() string { return m.jobName }

// EnvName returns the environment display name
func (m Model) EnvName() string { return m.envName }

func (m Model) helpView() string {
	help := styles.BoxStyle.Render(
		lipgloss.JoinVertical(lipgloss.Left,
			styles.TitleStyle.Render("Keyboard Shortcuts"),
			"",
			fmt.Sprintf("%s  %s", styles.HelpKeyStyle.Render("↑/k  ↓/j"), styles.HelpDescStyle.Render("Navigate builds")),
			fmt.Sprintf("%s      %s", styles.HelpKeyStyle.Render("Enter"), styles.HelpDescStyle.Render("Select")),
			fmt.Sprintf("%s        %s", styles.HelpKeyStyle.Render("b"), styles.HelpDescStyle.Render("Trigger build")),
			fmt.Sprintf("%s        %s", styles.HelpKeyStyle.Render("d"), styles.HelpDescStyle.Render("Trigger deploy")),
		fmt.Sprintf("%s        %s", styles.HelpKeyStyle.Render("l"), styles.HelpDescStyle.Render("Open selected build log")),
		fmt.Sprintf("%s        %s", styles.HelpKeyStyle.Render("f"), styles.HelpDescStyle.Render("Toggle favorite")),
		fmt.Sprintf("%s        %s", styles.HelpKeyStyle.Render("r"), styles.HelpDescStyle.Render("Refresh")),
			fmt.Sprintf("%s      %s", styles.HelpKeyStyle.Render("Esc"), styles.HelpDescStyle.Render("Go back")),
			fmt.Sprintf("%s        %s", styles.HelpKeyStyle.Render("?"), styles.HelpDescStyle.Render("Toggle help")),
			fmt.Sprintf("%s        %s", styles.HelpKeyStyle.Render("q"), styles.HelpDescStyle.Render("Quit")),
		),
	)
	return "\n" + lipgloss.NewStyle().PaddingLeft(2).Render(help)
}
