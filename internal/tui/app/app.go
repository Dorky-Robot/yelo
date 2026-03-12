package app

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dorkyrobot/yelo/internal/aws"
	"github.com/dorkyrobot/yelo/internal/config"
	"github.com/dorkyrobot/yelo/internal/state"
)

// Run starts the interactive TUI.
func Run(cfg *config.Config, st *state.State, client aws.S3Client, bucket string) error {
	m := NewModel(cfg, st, client, bucket)

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("running TUI: %w", err)
	}

	return nil
}
