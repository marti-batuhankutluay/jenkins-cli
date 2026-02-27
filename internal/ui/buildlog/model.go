package buildlog

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/marti-batuhankutluay/jenkins-cli/internal/jenkins"
	"github.com/marti-batuhankutluay/jenkins-cli/internal/ui/styles"
)

type BackMsg struct{}

type logChunkMsg struct {
	content   string
	nextStart int64
	moreData  bool
	err       error
}

type tickMsg time.Time

type Model struct {
	client      *jenkins.Client
	jobPath     string
	buildNumber int
	jobName     string
	envName     string
	viewport    viewport.Model
	content     strings.Builder
	textStart   int64
	moreData    bool
	loading     bool
	done        bool
	err         string
	width       int
	height      int
	autoScroll  bool
}

func New(client *jenkins.Client, jobPath string, buildNumber int, jobName, envName string) Model {
	vp := viewport.New(80, 30)
	vp.Style = styles.ItemStyle

	return Model{
		client:      client,
		jobPath:     jobPath,
		buildNumber: buildNumber,
		jobName:     jobName,
		envName:     envName,
		viewport:    vp,
		loading:     true,
		moreData:    true,
		autoScroll:  true,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.fetchChunk(), m.tick())
}

func (m Model) fetchChunk() tea.Cmd {
	start := m.textStart
	return func() tea.Msg {
		content, nextStart, more, err := m.client.GetBuildLogStream(m.jobPath, m.buildNumber, start)
		return logChunkMsg{
			content:   content,
			nextStart: nextStart,
			moreData:  more,
			err:       err,
		}
	}
}

func (m Model) tick() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var vpCmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = m.width - 2
		m.viewport.Height = m.height - 5
		return m, nil

	case logChunkMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}

		if msg.content != "" {
			m.content.WriteString(msg.content)
			m.viewport.SetContent(m.content.String())
			if m.autoScroll {
				m.viewport.GotoBottom()
			}
		}

		m.textStart = msg.nextStart
		m.moreData = msg.moreData

		if !msg.moreData {
			m.done = true
		}
		return m, nil

	case tickMsg:
		if m.moreData && !m.done {
			return m, tea.Batch(m.tick(), m.fetchChunk())
		}
		return m, m.tick() // keep ticking for auto-scroll check

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "esc", "backspace":
			return m, func() tea.Msg { return BackMsg{} }
		case "a":
			m.autoScroll = !m.autoScroll
			if m.autoScroll {
				m.viewport.GotoBottom()
			}
			return m, nil
		case "G":
			m.viewport.GotoBottom()
			return m, nil
		case "g":
			m.viewport.GotoTop()
			return m, nil
		}
	}

	// Let viewport handle scroll keys
	m.viewport, vpCmd = m.viewport.Update(msg)

	// If user scrolled up, disable auto-scroll
	if !m.viewport.AtBottom() {
		m.autoScroll = false
	}

	return m, vpCmd
}

func (m Model) View() string {
	if m.height == 0 {
		return ""
	}

	header := styles.HeaderStyle.Width(m.width).Render(
		fmt.Sprintf(" jenkins-cli  %s  %s  Build #%d", m.envName, m.jobName, m.buildNumber),
	)

	scrollPct := fmt.Sprintf("%3.f%%", m.viewport.ScrollPercent()*100)
	footer := styles.FooterStyle.Width(m.width).Render(
		styles.HelpKeyStyle.Render("↑↓/PgUp/PgDn")+" "+styles.HelpDescStyle.Render("scroll")+
			"  "+styles.HelpKeyStyle.Render("g/G")+" "+styles.HelpDescStyle.Render("top/bottom")+
			"  "+styles.HelpKeyStyle.Render("a")+" "+styles.HelpDescStyle.Render("auto-scroll")+
			"  "+styles.HelpKeyStyle.Render("Esc")+" "+styles.HelpDescStyle.Render("back")+
			"  "+strings.Repeat(" ", max(0, m.width-68))+scrollPct,
	)

	// Status line (1 row)
	var statusLine string
	if m.err != "" {
		statusLine = styles.ErrorStyle.PaddingLeft(2).Render("✗ Error: " + m.err)
	} else if m.loading {
		statusLine = styles.RunningStyle.PaddingLeft(2).Render("⟳ Loading log...")
	} else if m.done {
		statusLine = styles.SuccessStyle.PaddingLeft(2).Render("✓ Build complete")
	} else {
		statusLine = styles.RunningStyle.PaddingLeft(2).Render("⟳ Streaming log...")
	}
	if m.autoScroll {
		statusLine += styles.MutedStyle.Render("  [auto-scroll ON]")
	} else {
		statusLine += styles.WarningStyle.Render("  [auto-scroll OFF]")
	}

	// Resize viewport to fill: total - header(1) - statusline(1) - footer(1)
	vpHeight := m.height - 3
	if vpHeight < 1 {
		vpHeight = 1
	}
	m.viewport.Width = m.width
	m.viewport.Height = vpHeight

	return header + "\n" + statusLine + "\n" + m.viewport.View() + "\n" + footer
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
