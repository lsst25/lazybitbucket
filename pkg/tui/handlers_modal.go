package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/tui/components"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

// handleModalSelected dispatches a modal pick through the pending callback.
// The URL picker is the only thing that sets one.
func (a *App) handleModalSelected(msg components.ModalSelectedMsg) (tea.Model, tea.Cmd) {
	fn := a.onSelect
	a.onSelect = nil
	if fn != nil {
		return a, fn(msg.Item)
	}
	return a, nil
}

// handleModalCancelled drops the pending callback.
func (a *App) handleModalCancelled() (tea.Model, tea.Cmd) {
	a.onSelect = nil
	return a, nil
}

// handleExpandBlock shows a truncated detail block in a read-only modal.
func (a *App) handleExpandBlock(msg views.ExpandBlockMsg) (tea.Model, tea.Cmd) {
	items := make([]components.ModalItem, 0, len(msg.Lines))
	for _, line := range msg.Lines {
		items = append(items, components.ModalItem{Label: line})
	}
	a.modal.SetSize(a.width, a.height-1)
	a.modal.ShowReadOnly(msg.Title, items)
	return a, nil
}
