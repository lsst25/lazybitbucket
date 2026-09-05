package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/lsst25/lazybitbucket/pkg/tui/components"
	"github.com/lsst25/lazybitbucket/pkg/tui/views"
)

// clearOverlayCallbacks drops every pending overlay result slot. A slot is
// armed only while its overlay is visible, so the moment any overlay answers —
// or is cancelled — none of them may still fire.
func (a *App) clearOverlayCallbacks() {
	a.onSelect = nil
	a.onChecklist = nil
	a.onForm = nil
	a.onFormID = ""
	a.onInput = nil
}

// handleModalSelected dispatches a modal pick through the pending callback.
func (a *App) handleModalSelected(msg components.ModalSelectedMsg) (tea.Model, tea.Cmd) {
	fn := a.onSelect
	a.clearOverlayCallbacks()
	if fn != nil {
		return a, fn(msg.Item)
	}
	return a, nil
}

// handleModalCancelled drops the pending callbacks.
func (a *App) handleModalCancelled() (tea.Model, tea.Cmd) {
	a.clearOverlayCallbacks()
	return a, nil
}

// handleChecklistConfirmed dispatches a confirmed multi-select — the reviewer
// picker — through the pending callback.
func (a *App) handleChecklistConfirmed(msg components.ChecklistConfirmedMsg) (tea.Model, tea.Cmd) {
	fn := a.onChecklist
	a.clearOverlayCallbacks()
	if fn != nil {
		return a, fn(msg.Selected)
	}
	return a, nil
}

// handleFormConfirmed dispatches a confirmed form through the pending callback.
// A result whose id is not the one the slot was armed for belongs to a dialog
// that is already gone: it is dropped without clearing, so the callback of the
// form that is armed now survives it.
func (a *App) handleFormConfirmed(msg components.FormConfirmedMsg) (tea.Model, tea.Cmd) {
	if msg.ID != a.onFormID {
		return a, nil
	}
	fn := a.onForm
	a.clearOverlayCallbacks()
	if fn != nil {
		return a, fn(msg)
	}
	return a, nil
}

// handleFormCancelled drops the pending callbacks, unless the cancellation came
// from a form other than the armed one.
func (a *App) handleFormCancelled(msg components.FormCancelledMsg) (tea.Model, tea.Cmd) {
	if msg.ID != a.onFormID {
		return a, nil
	}
	a.clearOverlayCallbacks()
	return a, nil
}

// handleInputConfirmed dispatches confirmed input text through the pending
// callback.
func (a *App) handleInputConfirmed(msg components.InputConfirmedMsg) (tea.Model, tea.Cmd) {
	fn := a.onInput
	a.clearOverlayCallbacks()
	if fn != nil {
		return a, fn(msg.Text)
	}
	return a, nil
}

// handleInputCancelled drops the pending callbacks.
func (a *App) handleInputCancelled() (tea.Model, tea.Cmd) {
	a.clearOverlayCallbacks()
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
