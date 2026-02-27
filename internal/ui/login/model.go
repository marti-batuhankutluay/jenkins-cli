package login

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/marti-batuhankutluay/jenkins-cli/internal/config"
	"github.com/marti-batuhankutluay/jenkins-cli/internal/jenkins"
	"github.com/marti-batuhankutluay/jenkins-cli/internal/ui/styles"
)

type SavedMsg struct {
	Config *config.Config
}

type ValidateErrMsg struct {
	Err error
}

type field int

const (
	fieldURL field = iota
	fieldUser
	fieldToken
	fieldCount
)

type Model struct {
	inputs    [fieldCount]textinput.Model
	focused   field
	err       string
	validating bool
	width     int
	height    int
}

func New() Model {
	m := Model{}

	labels := []string{"Jenkins URL", "Username", "API Token"}
	placeholders := []string{"https://jenkins.example.com", "your.name", "your-api-token"}

	for i := 0; i < int(fieldCount); i++ {
		t := textinput.New()
		t.Placeholder = placeholders[i]
		t.Width = 50
		if field(i) == fieldToken {
			t.EchoMode = textinput.EchoPassword
			t.EchoCharacter = '•'
		}
		_ = labels[i]
		m.inputs[i] = t
	}

	m.inputs[fieldURL].Focus()
	return m
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case ValidateErrMsg:
		m.validating = false
		m.err = "Connection failed: " + msg.Err.Error()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "tab", "down":
			m.inputs[m.focused].Blur()
			m.focused = (m.focused + 1) % fieldCount
			m.inputs[m.focused].Focus()
			return m, nil

		case "shift+tab", "up":
			m.inputs[m.focused].Blur()
			m.focused = (m.focused - 1 + fieldCount) % fieldCount
			m.inputs[m.focused].Focus()
			return m, nil

		case "enter":
			if m.focused < fieldCount-1 {
				m.inputs[m.focused].Blur()
				m.focused++
				m.inputs[m.focused].Focus()
				return m, nil
			}
			// Last field: validate & save
			return m, m.submit()
		}
	}

	var cmd tea.Cmd
	m.inputs[m.focused], cmd = m.inputs[m.focused].Update(msg)
	return m, cmd
}

func (m Model) View() string {
	if m.height == 0 {
		return ""
	}

	header := styles.HeaderStyle.Width(m.width).Render(" jenkins-cli  Jenkins TUI")
	footer := styles.FooterStyle.Width(m.width).Render(
		styles.HelpKeyStyle.Render("Tab")+" "+styles.HelpDescStyle.Render("next field")+
			"  "+styles.HelpKeyStyle.Render("Enter")+" "+styles.HelpDescStyle.Render("confirm")+
			"  "+styles.HelpKeyStyle.Render("q")+" "+styles.HelpDescStyle.Render("quit"),
	)

	var rows []string
	rows = append(rows, "")
	rows = append(rows, lipgloss.NewStyle().Foreground(styles.ColorText).Bold(true).PaddingLeft(4).Render("Connect to Jenkins"))
	rows = append(rows, styles.MutedStyle.PaddingLeft(4).Render("Enter your credentials to get started"))
	rows = append(rows, "")

	labels := []string{"Jenkins URL", "Username", "API Token"}
	for i := 0; i < int(fieldCount); i++ {
		rows = append(rows, styles.InputLabelStyle.PaddingLeft(4).Render(labels[i]))
		inputBox := lipgloss.NewStyle().PaddingLeft(4)
		if field(i) == m.focused {
			inputBox = inputBox.Foreground(styles.ColorSecondary)
		}
		rows = append(rows, inputBox.Render(m.inputs[i].View()))
		rows = append(rows, "")
	}

	if m.err != "" {
		rows = append(rows, styles.ErrorStyle.PaddingLeft(4).Render("✗ "+m.err))
		rows = append(rows, "")
	}
	if m.validating {
		rows = append(rows, styles.RunningStyle.PaddingLeft(4).Render("⟳ Connecting..."))
	}

	body := strings.Join(rows, "\n")
	bodyHeight := m.height - 2
	if bodyHeight < 0 {
		bodyHeight = 0
	}
	lines := strings.Count(body, "\n")
	if pad := bodyHeight - lines; pad > 0 {
		body += strings.Repeat("\n", pad)
	}

	return header + "\n" + body + "\n" + footer
}

func (m *Model) submit() tea.Cmd {
	u := strings.TrimSpace(m.inputs[fieldURL].Value())
	user := strings.TrimSpace(m.inputs[fieldUser].Value())
	token := strings.TrimSpace(m.inputs[fieldToken].Value())

	if u == "" || user == "" || token == "" {
		m.err = "All fields are required"
		return nil
	}

	m.validating = true
	m.err = ""

	cfg := &config.Config{
		JenkinsURL: u,
		Username:   user,
		APIToken:   token,
	}

	return func() tea.Msg {
		client := jenkins.NewClient(u, user, token)
		if err := client.Validate(); err != nil {
			return ValidateErrMsg{Err: err}
		}
		if err := config.Save(cfg); err != nil {
			return ValidateErrMsg{Err: err}
		}
		return SavedMsg{Config: cfg}
	}
}
