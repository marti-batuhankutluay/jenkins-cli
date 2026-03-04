package ui

import (
	"errors"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/marti-batuhankutluay/jenkins-cli/internal/config"
	"github.com/marti-batuhankutluay/jenkins-cli/internal/favorites"
	"github.com/marti-batuhankutluay/jenkins-cli/internal/jenkins"
	"github.com/marti-batuhankutluay/jenkins-cli/internal/ui/activebuilds"
	"github.com/marti-batuhankutluay/jenkins-cli/internal/ui/buildlog"
	"github.com/marti-batuhankutluay/jenkins-cli/internal/ui/envlist"
	"github.com/marti-batuhankutluay/jenkins-cli/internal/ui/jobdetail"
	"github.com/marti-batuhankutluay/jenkins-cli/internal/ui/joblist"
	"github.com/marti-batuhankutluay/jenkins-cli/internal/ui/login"
)

type screen int

const (
	screenLogin screen = iota
	screenEnvList
	screenJobList
	screenJobDetail
	screenBuildLog
	screenActiveBuilds
)

type screenEntry struct {
	kind  screen
	model tea.Model
}

type App struct {
	stack  []screenEntry
	client *jenkins.Client
	favs   *favorites.Favorites
	width  int
	height int
}

func NewApp(cfg *config.Config) *App {
	app := &App{}
	favs, _ := favorites.Load()
	if favs == nil {
		favs = &favorites.Favorites{}
	}
	app.favs = favs
	if cfg != nil {
		app.client = jenkins.NewClient(cfg.JenkinsURL, cfg.Username, cfg.APIToken)
		app.stack = []screenEntry{{kind: screenEnvList, model: envlist.New(app.client)}}
	} else {
		app.stack = []screenEntry{{kind: screenLogin, model: login.New()}}
	}
	return app
}

func (a *App) current() *screenEntry {
	if len(a.stack) == 0 {
		return nil
	}
	return &a.stack[len(a.stack)-1]
}

// pushSized pushes a new model onto the stack, pre-sizes it with the current
// terminal dimensions, and returns its Init command.
func (a *App) pushSized(kind screen, model tea.Model) tea.Cmd {
	sized, _ := model.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})
	a.stack = append(a.stack, screenEntry{kind: kind, model: sized})
	return sized.Init()
}

func (a *App) pop() bool {
	if len(a.stack) > 1 {
		a.stack = a.stack[:len(a.stack)-1]
		return true
	}
	return false
}

func (a App) Init() tea.Cmd {
	cur := a.current()
	if cur == nil {
		return nil
	}
	return cur.model.Init()
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return a, tea.Quit
		}
	}

	cur := a.current()
	if cur == nil {
		return a, nil
	}

	newModel, cmd := cur.model.Update(msg)
	cur.model = newModel

	// --- Auth error: token expired / invalid → back to login ---
	switch m := msg.(type) {
	case envlist.ErrMsg:
		if errors.Is(m.Err, jenkins.ErrUnauthorized) {
			return a, a.redirectToLogin()
		}
	case joblist.ErrMsg:
		if errors.Is(m.Err, jenkins.ErrUnauthorized) {
			return a, a.redirectToLogin()
		}
	case jobdetail.ErrMsg:
		if errors.Is(m.Err, jenkins.ErrUnauthorized) {
			return a, a.redirectToLogin()
		}
	}

	// --- Navigation routing ---

	switch m := msg.(type) {
	// Login succeeded → replace with EnvList
	case login.SavedMsg:
		a.client = jenkins.NewClient(m.Config.JenkinsURL, m.Config.Username, m.Config.APIToken)
		envModel := envlist.New(a.client)
		sized, _ := envModel.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})
		a.stack[len(a.stack)-1] = screenEntry{kind: screenEnvList, model: sized}
		return a, sized.Init()

	// EnvList: selected an environment → push JobList
	case envlist.SelectedMsg:
		return a, a.pushSized(screenJobList, joblist.New(a.client, m.Job.Name, m.Job.Name))

	// EnvList: selected a favorite → jump straight to its JobList
	case envlist.FavoriteSelectedMsg:
		return a, a.pushSized(screenJobList,
			joblist.NewWithEnv(a.client, m.Fav.JobPath, m.Fav.Name, m.Fav.EnvName))

	// Any screen: toggle favorite → persist, notify current screen, refresh envlist
	case favorites.ToggleFavoriteMsg:
		added, _ := a.favs.Toggle(m.Fav)
		result := favorites.FavToggledMsg{Added: added, Name: m.Fav.Name}
		// Send result to current screen for inline notification
		newModel, cmd := cur.model.Update(result)
		cur.model = newModel
		// Also refresh envlist favorites state
		a.forwardToEnvList(favorites.ToggleFavoriteMsg{Fav: m.Fav})
		return a, cmd

	// JobList: folder → push another JobList; job → push JobDetail
	case joblist.SelectedMsg:
		if m.IsFolder {
			envName := a.currentEnvName()
			return a, a.pushSized(screenJobList, joblist.NewWithEnv(a.client, m.JobPath, m.Job.Name, envName))
		}
		return a, a.pushSized(screenJobDetail,
			jobdetail.New(a.client, m.JobPath, m.Job.Name, a.currentEnvName()))

	// Any screen: open active builds overlay
	case activebuilds.OpenMsg:
		return a, a.pushSized(screenActiveBuilds, activebuilds.New(a.client))

	// ActiveBuilds: open log for a running build
	case activebuilds.OpenLogMsg:
		return a, a.pushSized(screenBuildLog,
			buildlog.New(a.client, m.JobPath, m.BuildNumber, m.JobName, m.EnvName))

	// JobDetail: open build log
	case jobdetail.OpenLogMsg:
		envName, jobName := a.currentEnvJobName()
		return a, a.pushSized(screenBuildLog,
			buildlog.New(a.client, m.JobPath, m.BuildNumber, jobName, envName))

	// Back from any screen
	case joblist.BackMsg, jobdetail.BackMsg, buildlog.BackMsg, activebuilds.BackMsg:
		if a.pop() {
			if cur := a.current(); cur != nil {
				refreshed, refreshCmd := cur.model.Update(
					tea.WindowSizeMsg{Width: a.width, Height: a.height},
				)
				cur.model = refreshed
				return a, refreshCmd
			}
		}
		return a, nil
	}

	return a, cmd
}

func (a App) View() string {
	cur := a.current()
	if cur == nil {
		return ""
	}
	return cur.model.View()
}

func (a *App) redirectToLogin() tea.Cmd {
	_ = config.Delete()
	a.client = nil
	loginModel := login.New()
	sized, _ := loginModel.Update(tea.WindowSizeMsg{Width: a.width, Height: a.height})
	a.stack = []screenEntry{{kind: screenLogin, model: sized}}
	return sized.Init()
}

// forwardToEnvList sends a message directly to the envlist model at the bottom of the stack.
func (a *App) forwardToEnvList(msg tea.Msg) tea.Cmd {
	for i := range a.stack {
		if a.stack[i].kind == screenEnvList {
			updated, cmd := a.stack[i].model.Update(msg)
			a.stack[i].model = updated
			return cmd
		}
	}
	return nil
}

func (a *App) currentEnvName() string {
	for i := 0; i < len(a.stack); i++ {
		if a.stack[i].kind == screenJobList {
			if m, ok := a.stack[i].model.(joblist.Model); ok {
				return m.FolderName()
			}
		}
	}
	return ""
}

func (a *App) currentEnvJobName() (envName, jobName string) {
	for i := len(a.stack) - 1; i >= 0; i-- {
		if a.stack[i].kind == screenJobDetail {
			if m, ok := a.stack[i].model.(jobdetail.Model); ok {
				return m.EnvName(), m.JobName()
			}
		}
	}
	return "", ""
}
