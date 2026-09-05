package components

import (
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lsst25/lazybitbucket/pkg/tui/theme"
)

// FormFieldKind is the kind of a single row in a FormModal.
type FormFieldKind int

const (
	// FormChoice cycles through Options with h/l/left/right and space.
	FormChoice FormFieldKind = iota
	// FormToggle flips On with space.
	FormToggle
	// FormText holds a value edited inline with a TextInput on enter.
	FormText
	// FormNote is a read-only line, never selectable.
	FormNote
)

// FormField is one row of a FormModal.
type FormField struct {
	ID      string
	Label   string
	Kind    FormFieldKind
	Options []string // FormChoice: display labels, Index selects one
	Index   int      // FormChoice: index into Options
	On      bool     // FormToggle
	Value   string   // FormText: edited inline, enter keeps and esc reverts
	Note    string   // FormNote: read-only line, not selectable
}

// FormConfirmedMsg is sent when the user confirms the form, carrying the final field values.
type FormConfirmedMsg struct {
	ID     string
	Fields []FormField
}

// FormCancelledMsg is sent when the user cancels the form.
type FormCancelledMsg struct{ ID string }

const (
	formMaxWidth   = 60
	formMinWidth   = 20
	formLabelGap   = 2
	formMinPopupH  = 5
	formChromeRows = 4 // title, blank, blank, footer
)

// FormModal is a small typed form overlay: choice, toggle, text and note rows
// with a confirm footer. It implements OverlayPanel.
type FormModal struct {
	id           string
	title        string
	confirmLabel string
	fields       []FormField
	cursor       int
	visible      bool
	editing      bool
	editInput    TextInput
	editOriginal string
	width        int
	height       int
}

// NewFormModal returns a hidden form modal.
func NewFormModal() FormModal {
	return FormModal{cursor: -1}
}

// Show opens the form with the given id, title, confirm label and fields.
// The fields are copied, so the caller's slice is never mutated.
func (f *FormModal) Show(id, title, confirmLabel string, fields []FormField) {
	f.id = id
	f.title = title
	f.confirmLabel = confirmLabel
	f.fields = slices.Clone(fields)
	f.clampChoices()
	f.cursor = f.firstSelectable()
	f.visible = true
	f.editing = false
	f.editOriginal = ""
	f.editInput = NewTextInput()
}

// Hide closes the form without emitting a message.
func (f *FormModal) Hide() { f.visible = false }

// IsVisible reports whether the form is open.
func (f *FormModal) IsVisible() bool { return f.visible }

// IsEditing reports whether a text field is currently being edited inline.
func (f *FormModal) IsEditing() bool { return f.visible && f.editing }

// ID returns the id of the form currently shown.
func (f *FormModal) ID() string { return f.id }

// SetSize records the terminal size used for centering and width.
func (f *FormModal) SetSize(w, h int) {
	f.width = w
	f.height = h
}

func (f *FormModal) clampChoices() {
	for i := range f.fields {
		if f.fields[i].Kind != FormChoice {
			continue
		}
		n := len(f.fields[i].Options)
		if n == 0 || f.fields[i].Index < 0 || f.fields[i].Index >= n {
			f.fields[i].Index = 0
		}
	}
}

func (f *FormModal) firstSelectable() int {
	for i, field := range f.fields {
		if field.Kind != FormNote {
			return i
		}
	}
	return -1
}

// moveCursor advances the cursor by delta, skipping note rows and wrapping
// around like Modal.moveCursor does over separators.
func (f *FormModal) moveCursor(delta int) {
	n := len(f.fields)
	if n == 0 {
		return
	}
	if f.cursor < 0 {
		f.cursor = f.firstSelectable()
		return
	}
	cur := f.cursor
	for range n {
		cur += delta
		switch {
		case cur < 0:
			cur = n - 1
		case cur >= n:
			cur = 0
		}
		if f.fields[cur].Kind != FormNote {
			f.cursor = cur
			return
		}
	}
}

func (f *FormModal) currentField() *FormField {
	if f.cursor < 0 || f.cursor >= len(f.fields) {
		return nil
	}
	return &f.fields[f.cursor]
}

// cycleChoice moves the selected option of the cursor row by delta, wrapping.
func (f *FormModal) cycleChoice(delta int) {
	field := f.currentField()
	if field == nil || field.Kind != FormChoice {
		return
	}
	n := len(field.Options)
	if n == 0 {
		return
	}
	field.Index = ((field.Index+delta)%n + n) % n
}

// toggleOrCycle flips a toggle or advances a choice, per the space key.
func (f *FormModal) toggleOrCycle() {
	field := f.currentField()
	if field == nil {
		return
	}
	switch field.Kind {
	case FormToggle:
		field.On = !field.On
	case FormChoice:
		f.cycleChoice(1)
	case FormText, FormNote:
	default:
	}
}

// Update handles key and mouse messages while the form is visible.
func (f *FormModal) Update(msg tea.Msg) (FormModal, tea.Cmd) {
	if !f.visible {
		return *f, nil
	}
	if km, ok := msg.(tea.KeyMsg); ok {
		if f.editing {
			return f.handleEditKey(km)
		}
		return f.handleKey(km)
	}
	return *f, nil
}

func (f *FormModal) handleKey(msg tea.KeyMsg) (FormModal, tea.Cmd) {
	switch msg.String() {
	case "j", keyDown, "tab":
		f.moveCursor(1)
	case "k", "up":
		f.moveCursor(-1)
	case "l", "right":
		f.cycleChoice(1)
	case "h", "left":
		f.cycleChoice(-1)
	case " ":
		f.toggleOrCycle()
	case keyEnter:
		return f.handleEnter()
	case keyEsc, "q":
		return f.cancel()
	}
	return *f, nil
}

// handleEnter starts inline editing on a text row and confirms everywhere else.
func (f *FormModal) handleEnter() (FormModal, tea.Cmd) {
	field := f.currentField()
	if field != nil && field.Kind == FormText {
		f.editing = true
		f.editOriginal = field.Value
		f.editInput = NewTextInput()
		f.editInput.SetValue(field.Value)
		return *f, nil
	}
	return f.confirm()
}

func (f *FormModal) confirm() (FormModal, tea.Cmd) {
	f.visible = false
	f.editing = false
	msg := FormConfirmedMsg{ID: f.id, Fields: slices.Clone(f.fields)}
	return *f, func() tea.Msg { return msg }
}

func (f *FormModal) cancel() (FormModal, tea.Cmd) {
	f.visible = false
	f.editing = false
	id := f.id
	return *f, func() tea.Msg { return FormCancelledMsg{ID: id} }
}

// handleEditKey routes every key but enter and esc into the inline TextInput,
// so q, j, k, h, l and space insert characters while editing.
func (f *FormModal) handleEditKey(msg tea.KeyMsg) (FormModal, tea.Cmd) {
	field := f.currentField()
	if field == nil {
		f.editing = false
		return *f, nil
	}
	switch msg.Type {
	case tea.KeyEnter:
		field.Value = f.editInput.Value()
		f.editing = false
	case tea.KeyEsc:
		field.Value = f.editOriginal
		f.editing = false
	default:
		updated, _ := f.editInput.Update(msg)
		f.editInput = updated
	}
	return *f, nil
}

// contentWidth is min(60, width-4), never narrower than formMinWidth.
func (f *FormModal) contentWidth() int {
	return max(min(formMaxWidth, f.width-4), formMinWidth)
}

// labelWidth is the widest label, capped at half the popup so values stay visible.
func (f *FormModal) labelWidth(contentW int) int {
	w := 0
	for _, field := range f.fields {
		if field.Kind == FormNote {
			continue
		}
		w = max(w, lipgloss.Width(field.Label))
	}
	return min(w, max(contentW/2, 1))
}

// View renders the bordered popup: title, one line per field and a footer.
func (f *FormModal) View() string {
	if !f.visible {
		return ""
	}
	contentW := f.contentWidth()
	titleStyle := lipgloss.NewStyle().Foreground(theme.ColorAccent).Bold(true)
	footerStyle := lipgloss.NewStyle().Foreground(theme.ColorGray)

	lines := []string{" " + titleStyle.Render(TruncateEnd(f.title, contentW-1)), ""}
	lines = append(lines, f.renderRows(contentW)...)
	lines = append(lines, "", " "+footerStyle.Render(TruncateEnd(f.footer(), contentW-1)))
	lines = f.clampHeight(lines)

	borderStyle := lipgloss.NewStyle().Foreground(theme.ColorAccent)
	bv := borderStyle.Render("│")

	var body strings.Builder
	body.WriteString(borderStyle.Render("╭"+strings.Repeat("─", contentW)+"╮") + "\n")
	for _, line := range lines {
		if pad := contentW - lipgloss.Width(line); pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		body.WriteString(bv + line + bv + "\n")
	}
	body.WriteString(borderStyle.Render("╰" + strings.Repeat("─", contentW) + "╯"))

	return body.String()
}

// clampHeight drops field rows so the popup fits the terminal height.
func (f *FormModal) clampHeight(lines []string) []string {
	maxLines := max(f.height-2, formMinPopupH)
	if len(lines) <= maxLines {
		return lines
	}
	keep := max(maxLines-formChromeRows, 0)
	trimmed := make([]string, 0, maxLines)
	trimmed = append(trimmed, lines[0], lines[1])
	trimmed = append(trimmed, lines[2:2+keep]...)
	trimmed = append(trimmed, lines[len(lines)-2], lines[len(lines)-1])
	return trimmed
}

func (f *FormModal) footer() string {
	if f.editing {
		return "enter: keep   esc: revert"
	}
	if field := f.currentField(); field != nil && field.Kind == FormText {
		// Enter on a text row opens the editor; the form confirms from any other row.
		return "enter: edit   j/k: other rows   esc: cancel"
	}
	return "enter: " + f.confirmLabel + "   esc: cancel"
}

func (f *FormModal) renderRows(contentW int) []string {
	labelW := f.labelWidth(contentW)
	noteStyle := lipgloss.NewStyle().Foreground(theme.ColorGray)
	rowStyle := lipgloss.NewStyle().Width(contentW)
	cursorStyle := rowStyle.Bold(true).Background(theme.ColorHighlight)

	lines := make([]string, 0, len(f.fields))
	for i, field := range f.fields {
		if field.Kind == FormNote {
			lines = append(lines, rowStyle.Render(noteStyle.Render(" "+TruncateEnd(field.Note, contentW-1))))
			continue
		}
		label := TruncateEnd(field.Label, labelW)
		if pad := labelW - lipgloss.Width(label); pad > 0 {
			label += strings.Repeat(" ", pad)
		}
		valueW := max(contentW-labelW-formLabelGap-2, 1)

		if f.editing && i == f.cursor {
			f.editInput.SetWidth(valueW)
			line := " " + label + strings.Repeat(" ", formLabelGap) + f.editInput.View()
			lines = append(lines, cursorStyle.Render(line))
			continue
		}

		line := " " + label + strings.Repeat(" ", formLabelGap) + TruncateEnd(fieldValue(field), valueW)
		if i == f.cursor {
			lines = append(lines, cursorStyle.Render(line))
		} else {
			lines = append(lines, rowStyle.Render(line))
		}
	}
	return lines
}

// fieldValue is the value column of a row: ‹ option ›, [x] or the raw text.
func fieldValue(field FormField) string {
	switch field.Kind {
	case FormChoice:
		if len(field.Options) == 0 {
			return "‹ ›"
		}
		return "‹ " + field.Options[field.Index] + " ›"
	case FormToggle:
		if field.On {
			return "[x]"
		}
		return "[ ]"
	case FormText:
		return field.Value
	case FormNote:
		return field.Note
	default:
		return field.Value
	}
}

// Intercept handles a message if the form is visible and implements OverlayPanel.
func (f *FormModal) Intercept(msg tea.Msg) (tea.Cmd, bool) {
	if !f.visible {
		return nil, false
	}
	switch msg.(type) {
	case tea.KeyMsg, tea.MouseMsg:
		updated, cmd := f.Update(msg)
		*f = updated
		return cmd, true
	}
	return nil, false
}

// Render draws the form centered on bg and implements OverlayPanel.
func (f *FormModal) Render(bg string, w, h int) string {
	if !f.visible {
		return bg
	}
	return centerOverlay(bg, f.View(), w, h)
}
