package components

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lsst25/lazybitbucket/pkg/internal/testkit"
)

// FormModal must stay usable in an OverlayStack.
var _ OverlayPanel = (*FormModal)(nil)

// mergeFormFields is the merge dialog shape: a note, a choice, a toggle and a text row.
func mergeFormFields() []FormField {
	return []FormField{
		{ID: "note", Kind: FormNote, Note: testFormNoteText},
		{
			ID:      testFieldStrategy,
			Label:   "Strategy",
			Kind:    FormChoice,
			Options: []string{testOptMergeCommit, testOptSquash, testOptFastFwd},
		},
		{ID: testFieldClose, Label: "Close source branch", Kind: FormToggle, On: true},
		{ID: testFieldMessage, Label: "Message", Kind: FormText, Value: testFormMessage},
	}
}

func showMergeForm() FormModal {
	f := NewFormModal()
	f.SetSize(80, 24)
	f.Show(testFormID, testFormTitle, testConfirmLabel, mergeFormFields())
	return f
}

func runeMsg(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func fieldByID(t *testing.T, fields []FormField, id string) FormField {
	t.Helper()
	for _, field := range fields {
		if field.ID == id {
			return field
		}
	}
	t.Fatalf("field %q not present in %+v", id, fields)
	return FormField{}
}

func TestFormModal_ShowStartsOnFirstSelectableRow(t *testing.T) {
	t.Parallel()
	f := showMergeForm()
	testkit.AssertEqual(t, "visible", f.IsVisible(), true)
	testkit.AssertEqual(t, "cursor skips the leading note", f.cursor, 1)
	f.Hide()
	testkit.AssertEqual(t, "hidden after Hide", f.IsVisible(), false)
}

func TestFormModal_NavigationSkipsNotesAndWraps(t *testing.T) {
	t.Parallel()
	f := showMergeForm()

	f, _ = f.Update(runeMsg('j'))
	testkit.AssertEqual(t, "j moves to the toggle", f.cursor, 2)
	f, _ = f.Update(tea.KeyMsg{Type: tea.KeyTab})
	testkit.AssertEqual(t, "tab moves to the text row", f.cursor, 3)
	f, _ = f.Update(tea.KeyMsg{Type: tea.KeyDown})
	testkit.AssertEqual(t, "down wraps past the note back to the choice", f.cursor, 1)
	f, _ = f.Update(runeMsg('k'))
	testkit.AssertEqual(t, "k wraps backwards past the note", f.cursor, 3)
	f, _ = f.Update(tea.KeyMsg{Type: tea.KeyUp})
	testkit.AssertEqual(t, "up moves to the toggle", f.cursor, 2)
}

func TestFormModal_CycleChoiceWrapsBothWays(t *testing.T) {
	t.Parallel()
	f := showMergeForm()

	f, _ = f.Update(runeMsg('l'))
	testkit.AssertEqual(t, "l advances", f.fields[1].Index, 1)
	f, _ = f.Update(tea.KeyMsg{Type: tea.KeyRight})
	f, _ = f.Update(tea.KeyMsg{Type: tea.KeyRight})
	testkit.AssertEqual(t, "right wraps to the first option", f.fields[1].Index, 0)
	f, _ = f.Update(runeMsg('h'))
	testkit.AssertEqual(t, "h wraps to the last option", f.fields[1].Index, 2)
	f, _ = f.Update(tea.KeyMsg{Type: tea.KeyLeft})
	testkit.AssertEqual(t, "left steps back", f.fields[1].Index, 1)
}

func TestFormModal_SpaceTogglesAndCycles(t *testing.T) {
	t.Parallel()
	f := showMergeForm()

	f, _ = f.Update(tea.KeyMsg{Type: tea.KeySpace})
	testkit.AssertEqual(t, "space cycles a choice", f.fields[1].Index, 1)

	f, _ = f.Update(runeMsg('j'))
	f, _ = f.Update(tea.KeyMsg{Type: tea.KeySpace})
	testkit.AssertEqual(t, "space flips the toggle off", f.fields[2].On, false)
	f, _ = f.Update(tea.KeyMsg{Type: tea.KeySpace})
	testkit.AssertEqual(t, "space flips the toggle on", f.fields[2].On, true)
}

func TestFormModal_InlineTextEditKeepsValue(t *testing.T) {
	t.Parallel()
	f := showMergeForm()

	f, _ = f.Update(runeMsg('j'))
	f, _ = f.Update(runeMsg('j'))
	f, _ = f.Update(tea.KeyMsg{Type: tea.KeyEnter})
	testkit.AssertEqual(t, "editing started", f.IsEditing(), true)
	testkit.AssertEqual(t, "form stays open while editing", f.IsVisible(), true)

	f, _ = f.Update(tea.KeyMsg{Type: tea.KeyLeft})
	f, _ = f.Update(runeMsg('X'))
	f, _ = f.Update(tea.KeyMsg{Type: tea.KeyEnter})

	testkit.AssertEqual(t, "editing finished", f.IsEditing(), false)
	testkit.AssertEqual(t, "value edited at the cursor", f.fields[3].Value, "abXc")
	testkit.AssertEqual(t, "enter did not confirm the form", f.IsVisible(), true)
}

func TestFormModal_InlineTextEditRevertsOnEsc(t *testing.T) {
	t.Parallel()
	f := showMergeForm()

	f, _ = f.Update(runeMsg('k'))
	f, _ = f.Update(tea.KeyMsg{Type: tea.KeyEnter})
	f, _ = f.Update(runeMsg('z'))
	f, cmd := f.Update(tea.KeyMsg{Type: tea.KeyEsc})

	testkit.AssertEqual(t, "value reverted", f.fields[3].Value, testFormMessage)
	testkit.AssertEqual(t, "editing finished", f.IsEditing(), false)
	testkit.AssertEqual(t, "esc did not cancel the form", f.IsVisible(), true)
	testkit.AssertEqual(t, "no message emitted", cmd == nil, true)
}

func TestFormModal_EditingSwallowsNavigationKeys(t *testing.T) {
	t.Parallel()
	f := showMergeForm()

	f, _ = f.Update(runeMsg('k'))
	f, _ = f.Update(tea.KeyMsg{Type: tea.KeyEnter})
	for _, r := range []rune{'q', 'j', 'k', 'h', 'l'} {
		f, _ = f.Update(runeMsg(r))
	}
	f, _ = f.Update(tea.KeyMsg{Type: tea.KeySpace})
	f, _ = f.Update(tea.KeyMsg{Type: tea.KeyEnter})

	testkit.AssertEqual(t, "navigation keys typed into the input", f.fields[3].Value, "abcqjkhl ")
	testkit.AssertEqual(t, "cursor never moved", f.cursor, 3)
}

func TestFormModal_ConfirmCarriesValuesAndID(t *testing.T) {
	t.Parallel()
	f := showMergeForm()

	f, _ = f.Update(runeMsg('l'))                   // strategy -> squash
	f, _ = f.Update(runeMsg('j'))                   // close row
	f, _ = f.Update(tea.KeyMsg{Type: tea.KeySpace}) // close -> false
	f, _ = f.Update(runeMsg('j'))                   // message row
	f, _ = f.Update(tea.KeyMsg{Type: tea.KeyEnter}) // start editing
	f, _ = f.Update(runeMsg('!'))                   // type
	f, _ = f.Update(tea.KeyMsg{Type: tea.KeyEnter}) // keep
	f, _ = f.Update(runeMsg('k'))                   // back to the toggle
	updated, cmd := f.Update(tea.KeyMsg{Type: tea.KeyEnter})

	testkit.AssertEqual(t, "form closed on confirm", updated.IsVisible(), false)
	if cmd == nil {
		t.Fatal("expected a confirm command")
	}
	msg, ok := cmd().(FormConfirmedMsg)
	if !ok {
		t.Fatalf("expected FormConfirmedMsg, got %T", cmd())
	}
	testkit.AssertEqual(t, "form id", msg.ID, testFormID)
	testkit.AssertEqual(t, "field count", len(msg.Fields), 4)

	strategy := fieldByID(t, msg.Fields, testFieldStrategy)
	testkit.AssertEqual(t, "chosen index", strategy.Index, 1)
	testkit.AssertEqual(t, "chosen option", strategy.Options[strategy.Index], testOptSquash)
	testkit.AssertEqual(t, "toggle value", fieldByID(t, msg.Fields, testFieldClose).On, false)
	testkit.AssertEqual(t, "text value", fieldByID(t, msg.Fields, testFieldMessage).Value, "abc!")
}

func TestFormModal_ShowCopiesFieldsFromCaller(t *testing.T) {
	t.Parallel()
	fields := mergeFormFields()
	f := NewFormModal()
	f.SetSize(80, 24)
	f.Show(testFormID, testFormTitle, testConfirmLabel, fields)

	f, _ = f.Update(runeMsg('l'))
	testkit.AssertEqual(t, "caller slice untouched", fields[1].Index, 0)
}

func TestFormModal_ShowClampsOutOfRangeChoiceIndex(t *testing.T) {
	t.Parallel()
	f := NewFormModal()
	f.SetSize(80, 24)
	f.Show(testFormID, testFormTitle, testConfirmLabel, []FormField{
		{ID: testFieldStrategy, Label: "Strategy", Kind: FormChoice, Options: []string{testOptSquash}, Index: 7},
	})
	testkit.AssertEqual(t, "index clamped", f.fields[0].Index, 0)

	f, _ = f.Update(runeMsg('l'))
	testkit.AssertEqual(t, "single option cycles in place", f.fields[0].Index, 0)
}

func TestFormModal_CancelCarriesID(t *testing.T) {
	t.Parallel()
	for _, key := range []tea.KeyMsg{{Type: tea.KeyEsc}, runeMsg('q')} {
		f := showMergeForm()
		updated, cmd := f.Update(key)
		testkit.AssertEqual(t, "form closed on cancel", updated.IsVisible(), false)
		if cmd == nil {
			t.Fatalf("expected a cancel command for %v", key)
		}
		msg, ok := cmd().(FormCancelledMsg)
		if !ok {
			t.Fatalf("expected FormCancelledMsg, got %T", cmd())
		}
		testkit.AssertEqual(t, "form id", msg.ID, testFormID)
	}
}

func TestFormModal_NotesOnlyFormConfirmsAndCancels(t *testing.T) {
	t.Parallel()
	notes := []FormField{{ID: "body", Kind: FormNote, Note: testFormNoteText}}

	f := NewFormModal()
	f.SetSize(80, 24)
	f.Show("draft", "Mark #101 as draft?", "confirm", notes)
	testkit.AssertEqual(t, "no selectable row", f.cursor, -1)

	f, _ = f.Update(runeMsg('j'))
	testkit.AssertEqual(t, "navigation is a no-op", f.cursor, -1)

	confirmed, cmd := f.Update(tea.KeyMsg{Type: tea.KeyEnter})
	testkit.AssertEqual(t, "closed on confirm", confirmed.IsVisible(), false)
	if _, ok := cmd().(FormConfirmedMsg); !ok {
		t.Fatalf("expected FormConfirmedMsg, got %T", cmd())
	}

	f.Show("draft", "Mark #101 as draft?", "confirm", notes)
	cancelled, cmd := f.Update(runeMsg('q'))
	testkit.AssertEqual(t, "closed on cancel", cancelled.IsVisible(), false)
	if _, ok := cmd().(FormCancelledMsg); !ok {
		t.Fatalf("expected FormCancelledMsg, got %T", cmd())
	}
}

func TestFormModal_UpdateOnHiddenFormIsANoOp(t *testing.T) {
	t.Parallel()
	f := NewFormModal()
	f.SetSize(80, 24)

	updated, cmd := f.Update(tea.KeyMsg{Type: tea.KeyEnter})
	testkit.AssertEqual(t, "no command", cmd == nil, true)
	testkit.AssertEqual(t, "still hidden", updated.IsVisible(), false)
	testkit.AssertEqual(t, "no view", updated.View(), "")

	updated, cmd = f.Update(runeMsg('j'))
	testkit.AssertEqual(t, "no command from navigation", cmd == nil, true)
	testkit.AssertEqual(t, "cursor untouched", updated.cursor, -1)
}

func TestFormModal_InterceptOnlyWhenVisible(t *testing.T) {
	t.Parallel()
	f := NewFormModal()
	f.SetSize(80, 24)

	key := runeMsg('j')
	if _, ok := f.Intercept(key); ok {
		t.Error("hidden form should not intercept")
	}

	f.Show(testFormID, testFormTitle, testConfirmLabel, mergeFormFields())
	cmd, ok := f.Intercept(key)
	testkit.AssertEqual(t, "visible form intercepts keys", ok, true)
	testkit.AssertEqual(t, "no command from navigation", cmd == nil, true)
	testkit.AssertEqual(t, "key was consumed by the form", f.cursor, 2)

	if _, ok := f.Intercept(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown}); !ok {
		t.Error("visible form should swallow mouse messages")
	}
	if _, ok := f.Intercept(tea.WindowSizeMsg{Width: 10, Height: 10}); ok {
		t.Error("non input messages should pass through")
	}
}

func TestFormModal_ViewShowsFieldsAndFooter(t *testing.T) {
	t.Parallel()
	f := showMergeForm()
	plain := stripANSI(f.View())

	for _, want := range []string{
		testFormTitle,
		testFormNoteText,
		"Strategy",
		"‹ " + testOptMergeCommit + " ›",
		"[x]",
		testFormMessage,
		"enter: " + testConfirmLabel + "   esc: cancel",
	} {
		if !strings.Contains(plain, want) {
			t.Errorf("view missing %q\n%s", want, plain)
		}
	}

	f, _ = f.Update(tea.KeyMsg{Type: tea.KeySpace})
	f, _ = f.Update(runeMsg('j'))
	f, _ = f.Update(tea.KeyMsg{Type: tea.KeySpace})
	plain = stripANSI(f.View())
	if !strings.Contains(plain, "‹ "+testOptSquash+" ›") || !strings.Contains(plain, "[ ]") {
		t.Errorf("view did not follow the field values\n%s", plain)
	}
}

func TestFormModal_ViewFooterSwitchesWhileEditing(t *testing.T) {
	t.Parallel()
	f := showMergeForm()
	f, _ = f.Update(runeMsg('k'))
	f, _ = f.Update(tea.KeyMsg{Type: tea.KeyEnter})

	plain := stripANSI(f.View())
	if !strings.Contains(plain, "enter: keep   esc: revert") {
		t.Errorf("editing footer missing\n%s", plain)
	}
}

func TestFormModal_ViewAtNarrowWidthTruncatesWithoutPanicking(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("very long label ", 8)
	f := NewFormModal()
	f.SetSize(40, 24)
	f.Show(testFormID, strings.Repeat("wide title ", 10), testConfirmLabel, []FormField{
		{ID: "note", Kind: FormNote, Note: long},
		{ID: testFieldStrategy, Label: long, Kind: FormChoice, Options: []string{strings.Repeat("opt", 20)}},
		{ID: testFieldMessage, Label: long, Kind: FormText, Value: long},
	})

	for _, line := range strings.Split(stripANSI(f.View()), "\n") {
		if w := lipgloss.Width(line); w != 38 {
			t.Fatalf("line width = %d, want 38: %q", w, line)
		}
	}
}

func TestFormModal_ViewAtTinySizeStaysWithinHeight(t *testing.T) {
	t.Parallel()
	f := NewFormModal()
	f.SetSize(10, 6)
	f.Show(testFormID, testFormTitle, testConfirmLabel, mergeFormFields())

	// The popup keeps Modal's floor of five content rows plus two borders.
	lines := strings.Split(f.View(), "\n")
	if len(lines) > 7 {
		t.Fatalf("popup height = %d lines, want at most 7", len(lines))
	}
	// The footer survives the clamp; at 20 columns it is truncated with an ellipsis.
	if !strings.Contains(stripANSI(f.View()), "enter: "+testConfirmLabel) {
		t.Error("footer dropped when the popup was clamped")
	}
}

func TestFormModal_RenderCentersPopupAndPassesBackgroundThrough(t *testing.T) {
	t.Parallel()
	f := NewFormModal()
	f.SetSize(80, 24)
	bg := testkit.BlankCanvas(80, 24)
	testkit.AssertEqual(t, "hidden form returns bg", f.Render(bg, 80, 24), bg)

	f.Show(testFormID, testFormTitle, testConfirmLabel, mergeFormFields())
	out := f.Render(bg, 80, 24)
	lines := strings.Split(out, "\n")
	testkit.AssertEqual(t, "canvas height preserved", len(lines), 24)

	top := -1
	for i, line := range lines {
		if strings.Contains(stripANSI(line), "╭") {
			top = i
			break
		}
	}
	if top <= 0 {
		t.Fatalf("popup top border not found below the first row (got row %d)", top)
	}
	plainTop := stripANSI(lines[top])
	left := strings.Index(plainTop, "╭")
	popupW := f.contentWidth() + 2
	testkit.AssertEqual(t, "horizontally centred", left, (80-popupW)/2)
	if !strings.Contains(stripANSI(out), testFormTitle) {
		t.Error("title missing from the rendered overlay")
	}
}
