package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/marti-batuhankutluay/jenkins-cli/internal/config"
	"github.com/marti-batuhankutluay/jenkins-cli/internal/ui"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "jenkins-cli: error loading config: %v\n", err)
		os.Exit(1)
	}

	app := ui.NewApp(cfg)

	p := tea.NewProgram(
		app,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "jenkins-cli: %v\n", err)
		os.Exit(1)
	}
}
