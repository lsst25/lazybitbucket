package views

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/config"
	"github.com/lsst25/lazybitbucket/pkg/tui/components"
	"github.com/lsst25/lazybitbucket/pkg/tui/theme"
)

// PullRequestSelectedMsg is emitted when the list cursor lands on a pull
// request, by keyboard or by mouse.
type PullRequestSelectedMsg struct {
	PullRequest *bitbucket.PullRequest
}

// Column glyphs and widths. minTitleWidth drives the narrow-list drop rule:
// columns are removed from the end of the configured order until the flexible
// title column is at least this wide.
const (
	minTitleWidth  = 12
	buildGlyphCol  = 1
	approvalsCol   = 5
	authorCol      = 12
	updatedCol     = 8
	branchCol      = 20
	stateCol       = 8
	minRepoCol     = 8
	repoColDivisor = 4
)

// Title-bar geometry, shared by buildTitle and the mouse hit-test.
const (
	tabPrefix = "[2] "
	tabSep    = " - "
	tabSepW   = 3
	// titleBorderW is the "╭─" the top border draws before the title.
	titleBorderW = 2
)

// PullRequestList is the [2] panel: the pull requests of the active tab,
// filtered by the search bar and by the selected repository.
type PullRequestList struct {
	components.ListBase
	items      []bitbucket.PullRequest
	all        []bitbucket.PullRequest
	filter     string
	repoFilter string
	tabs       []config.PullRequestTabConfig
	tab        int
	tabCache   map[int][]bitbucket.PullRequest
	fields     []string
	builds     map[string]bitbucket.CommitStatusState
	theme      *theme.Theme
}

// NewPullRequestList constructs an empty list on the default theme.
func NewPullRequestList() *PullRequestList {
	return &PullRequestList{theme: theme.Default}
}

// SetFields sets the column order, using config.Field* names.
func (m *PullRequestList) SetFields(fields []string) { m.fields = fields }

// SetTabs sets the configured tabs shown in the panel title.
func (m *PullRequestList) SetTabs(tabs []config.PullRequestTabConfig) { m.tabs = tabs }

// ActiveTab returns the configuration of the tab currently displayed.
func (m *PullRequestList) ActiveTab() config.PullRequestTabConfig {
	if m.tab >= 0 && m.tab < len(m.tabs) {
		return m.tabs[m.tab]
	}
	return config.PullRequestTabConfig{}
}

// GetTabIndex returns the index of the active tab.
func (m *PullRequestList) GetTabIndex() int { return m.tab }

// SetTabIndex switches to the given tab, showing its cached rows if any.
func (m *PullRequestList) SetTabIndex(idx int) {
	if idx < 0 || idx >= len(m.tabs) {
		return
	}
	m.tab = idx
	m.loadFromCache()
}

// NextTab moves to the following tab, wrapping around.
func (m *PullRequestList) NextTab() {
	if len(m.tabs) == 0 {
		return
	}
	m.tab = (m.tab + 1) % len(m.tabs)
	m.loadFromCache()
}

// PrevTab moves to the preceding tab, wrapping around.
func (m *PullRequestList) PrevTab() {
	if len(m.tabs) == 0 {
		return
	}
	m.tab = (m.tab + len(m.tabs) - 1) % len(m.tabs)
	m.loadFromCache()
}

// ClickTabAt switches tabs from a click on the title bar and reports whether
// the tab changed. It hit-tests against the window buildTitle actually draws,
// measured in display columns, so a click always lands on the label under the
// pointer. It goes through the same cache-loading path as NextTab, so a clicked
// tab shows its own rows rather than the previous tab's.
func (m *PullRequestList) ClickTabAt(x int) bool {
	if len(m.tabs) == 0 {
		return false
	}
	// An unsized panel has drawn nothing yet, so there is no window to test
	// against: fall back to the whole tab bar rather than a bogus narrow one.
	maxTitleW := 0
	if m.Width > 0 {
		contentWidth, _ := components.PanelDimensions(m.Width, m.Height)
		maxTitleW = contentWidth - 1
	}
	start, names := m.tabWindow(maxTitleW)

	// The top border prepends "╭─" before the title, so the first title column
	// is titleBorderW in from the panel's left edge.
	pos := titleBorderW + lipgloss.Width(tabPrefix)
	for i, name := range names {
		zoneEnd := pos + lipgloss.Width(name) + tabSepW
		if x >= pos && x < zoneEnd {
			if m.tab == start+i {
				return false
			}
			m.tab = start + i
			m.loadFromCache()
			return true
		}
		pos = zoneEnd
	}
	return false
}

// HasCachedTab reports whether the active tab has a cache entry. It says
// nothing about in-flight requests; the app tracks those itself.
func (m *PullRequestList) HasCachedTab() bool {
	if m.tabCache == nil {
		return false
	}
	_, ok := m.tabCache[m.tab]
	return ok
}

// SetPullRequests replaces the active tab's pull requests, keeping the cursor
// on the same pull request when it is still present.
func (m *PullRequestList) SetPullRequests(prs []bitbucket.PullRequest) {
	selectedKey := ""
	if sel := m.SelectedPullRequest(); sel != nil {
		selectedKey = sel.Key()
	}

	if m.tabCache == nil {
		m.tabCache = make(map[int][]bitbucket.PullRequest)
	}
	m.tabCache[m.tab] = prs

	m.all = prs
	m.applyFilter()

	if selectedKey != "" {
		m.SelectByKey(selectedKey)
	}
}

// SetPullRequestsForTab caches pull requests for a tab without changing what is
// displayed. It is how background prefetch fills the inactive tabs.
func (m *PullRequestList) SetPullRequestsForTab(tab int, prs []bitbucket.PullRequest) {
	if m.tabCache == nil {
		m.tabCache = make(map[int][]bitbucket.PullRequest)
	}
	m.tabCache[tab] = prs
}

// CurrentPullRequests returns the active tab's pull requests before filtering.
func (m *PullRequestList) CurrentPullRequests() []bitbucket.PullRequest { return m.all }

// TabPullRequests returns a tab's pull requests before the repository and
// search filters, nil for a tab that has not been loaded. It is how a caller
// looks for a pull request in a tab the user is not on — the active tab is in
// there too, because SetPullRequests caches what it displays.
func (m *PullRequestList) TabPullRequests(tab int) []bitbucket.PullRequest {
	if m.tabCache == nil {
		return nil
	}
	return m.tabCache[tab]
}

// PatchPullRequest replaces a pull request by key wherever it is held, keeping
// the cursor where the user left it. Background detail fetches use it.
func (m *PullRequestList) PatchPullRequest(pr *bitbucket.PullRequest) {
	if pr == nil {
		return
	}
	key := pr.Key()
	patch := func(prs []bitbucket.PullRequest) {
		for i := range prs {
			if prs[i].Key() == key {
				prs[i] = *pr
				return
			}
		}
	}
	patch(m.all)
	if m.tabCache != nil {
		if cached, ok := m.tabCache[m.tab]; ok {
			patch(cached)
		}
	}
	m.applyFilterKeepCursor()
}

// SetBuildState records the rolled-up build state for a pull request key, which
// drives the single-glyph build column.
func (m *PullRequestList) SetBuildState(key string, state bitbucket.CommitStatusState) {
	if key == "" {
		return
	}
	if m.builds == nil {
		m.builds = make(map[string]bitbucket.CommitStatusState)
	}
	m.builds[key] = state
}

// SelectByKey moves the cursor to the pull request with the given key and
// reports whether it was found among the visible rows.
func (m *PullRequestList) SelectByKey(key string) bool {
	for i := range m.items {
		if m.items[i].Key() == key {
			m.Cursor = i
			m.AdjustOffset()
			return true
		}
	}
	return false
}

// SelectedPullRequest returns a pointer into the visible rows, or nil when the
// list is empty.
func (m *PullRequestList) SelectedPullRequest() *bitbucket.PullRequest {
	if m.Cursor < 0 || m.Cursor >= len(m.items) {
		return nil
	}
	return &m.items[m.Cursor]
}

// SetFilter applies the search bar query.
func (m *PullRequestList) SetFilter(query string) {
	m.filter = query
	m.applyFilter()
}

// ClearFilter drops the search query and keeps the cursor on its pull request.
func (m *PullRequestList) ClearFilter() {
	m.filter = ""
	m.applyFilterKeepCursor()
}

// SetRepoFilter limits the list to one repository slug; "" shows every one.
func (m *PullRequestList) SetRepoFilter(slug string) {
	m.repoFilter = slug
	m.applyFilter()
}

// RepoFilter returns the active repository slug filter, "" when unset.
func (m *PullRequestList) RepoFilter() string { return m.repoFilter }

// ContentHeight is the natural panel height for the accordion layout.
func (m *PullRequestList) ContentHeight() int { return m.ListBase.ContentHeight(7) }

func (m *PullRequestList) loadFromCache() {
	if m.tabCache != nil {
		if cached, ok := m.tabCache[m.tab]; ok {
			m.all = cached
			m.applyFilter()
			return
		}
	}
	m.all = nil
	m.applyFilter()
}

func (m *PullRequestList) applyFilter() {
	items := m.all
	if m.repoFilter != "" {
		filtered := make([]bitbucket.PullRequest, 0, len(items))
		for _, pr := range items {
			if pr.RepoSlug() == m.repoFilter {
				filtered = append(filtered, pr)
			}
		}
		items = filtered
	}
	if m.filter != "" {
		q := strings.ToLower(m.filter)
		filtered := make([]bitbucket.PullRequest, 0, len(items))
		for _, pr := range items {
			if strings.Contains(searchHaystack(&pr), q) {
				filtered = append(filtered, pr)
			}
		}
		items = filtered
	}
	m.items = items
	m.Cursor = 0
	m.Offset = 0
	m.SetItemCount(len(m.items))
}

func (m *PullRequestList) applyFilterKeepCursor() {
	prevKey := ""
	if sel := m.SelectedPullRequest(); sel != nil {
		prevKey = sel.Key()
	}
	m.applyFilter()
	if prevKey != "" {
		m.SelectByKey(prevKey)
	}
}

// searchHaystack is what the search bar matches against: repository, "#id",
// title, author name and source branch, lowercased.
func searchHaystack(pr *bitbucket.PullRequest) string {
	var b strings.Builder
	b.WriteString(pr.RepoSlug())
	b.WriteString(" #")
	b.WriteString(strconv.Itoa(pr.ID))
	b.WriteByte(' ')
	b.WriteString(pr.Title)
	b.WriteByte(' ')
	b.WriteString(pr.Author.Name())
	b.WriteByte(' ')
	b.WriteString(pr.SourceBranch())
	return strings.ToLower(b.String())
}

// Init satisfies the bubbletea component shape; the list has no startup work.
func (m *PullRequestList) Init() tea.Cmd { return nil }

// Update moves the cursor and announces the newly selected pull request.
func (m *PullRequestList) Update(msg tea.Msg) (*PullRequestList, tea.Cmd) {
	if !m.Focused {
		return m, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		if m.KeyNav(key.String()) {
			return m, func() tea.Msg {
				return PullRequestSelectedMsg{PullRequest: m.SelectedPullRequest()}
			}
		}
	}
	return m, nil
}

// View renders the panel, collapsing to a single title bar at height 1.
func (m *PullRequestList) View() string {
	contentWidth, _ := components.PanelDimensions(m.Width, m.Height)
	// Top border is "╭─{title}{padding}╮"; -1 reserves the closing corner.
	maxTitleW := contentWidth - 1

	footer := ""
	if n := len(m.items); n > 0 {
		footer = fmt.Sprintf("%d of %d", m.Cursor+1, n)
	}

	if m.Height <= 1 {
		return components.RenderCollapsedBar(m.buildTitle(maxTitleW), footer, m.Width, m.Focused)
	}

	visible := m.VisibleRows()
	layout := m.columnLayout(contentWidth)

	var rows []string
	end := min(m.Offset+visible, len(m.items))
	for i := m.Offset; i < end; i++ {
		rows = append(rows, m.renderRow(&m.items[i], layout, contentWidth, i == m.Cursor))
	}

	scroll := &components.ScrollInfo{Total: len(m.items), Visible: visible, Offset: m.Offset}
	return components.RenderPanelFull(m.buildTitle(maxTitleW), footer, strings.Join(rows, "\n"),
		m.Width, visible, m.Focused, scroll)
}

// buildTitle renders "[2] " plus the tab bar, styling the window tabWindow
// picked.
func (m *PullRequestList) buildTitle(maxTitleW int) string {
	if len(m.tabs) == 0 {
		return tabPrefix + "Pull Requests"
	}

	activeStyle := lipgloss.NewStyle().Foreground(theme.ColorAccent).Bold(true)
	inactiveStyle := lipgloss.NewStyle().Foreground(theme.ColorWhite)
	sep := lipgloss.NewStyle().Foreground(theme.ColorGray).Render(tabSep)

	start, names := m.tabWindow(maxTitleW)
	labels := make([]string, len(names))
	for i, name := range names {
		if start+i == m.tab {
			labels[i] = activeStyle.Render(name)
		} else {
			labels[i] = inactiveStyle.Render(name)
		}
	}
	return tabPrefix + strings.Join(labels, sep)
}

// tabWindow resolves the tab labels drawn for a title budget of maxTitleW
// display columns: the index of the first one and the label text as rendered.
// When the tabs do not fit it is a contiguous sliding window that always
// contains the active tab, expanding leftward first for context and then
// filling any remaining budget rightward; a single label wider than the whole
// budget is truncated. Rendering and mouse hit-testing both go through it, so
// they cannot disagree about what is on screen.
func (m *PullRequestList) tabWindow(maxTitleW int) (start int, names []string) {
	names = make([]string, len(m.tabs))
	widths := make([]int, len(m.tabs))
	total := lipgloss.Width(tabPrefix)
	for i, t := range m.tabs {
		names[i] = t.Name
		widths[i] = lipgloss.Width(t.Name)
		if i > 0 {
			total += tabSepW
		}
		total += widths[i]
	}
	if maxTitleW <= 0 || total <= maxTitleW {
		return 0, names
	}

	budget := maxTitleW - lipgloss.Width(tabPrefix)
	active := m.tab
	if active < 0 || active >= len(m.tabs) {
		active = 0
	}
	if budget > 0 && widths[active] > budget {
		names[active] = components.TruncateEnd(names[active], budget)
		widths[active] = lipgloss.Width(names[active])
	}

	end := active
	start = active
	used := widths[active]
	for start > 0 {
		cost := widths[start-1] + tabSepW
		if used+cost > budget {
			break
		}
		start--
		used += cost
	}
	for end < len(m.tabs)-1 {
		cost := widths[end+1] + tabSepW
		if used+cost > budget {
			break
		}
		end++
		used += cost
	}
	return start, names[start : end+1]
}

// columnSpec ties a column's width to its renderer so the two cannot diverge.
// A zero width marks the flexible column, of which there is exactly one.
type columnSpec struct {
	id     string
	width  func(items []bitbucket.PullRequest, listWidth int) int
	render func(pr *bitbucket.PullRequest, width int, selected bool) string
}

// column is a resolved spec with the width it was given for this render pass.
type column struct {
	spec  columnSpec
	width int
}

func (m *PullRequestList) columnSpecs() map[string]columnSpec {
	return map[string]columnSpec{
		config.FieldRepo: {
			id: config.FieldRepo,
			width: func(items []bitbucket.PullRequest, listWidth int) int {
				longest := 0
				for i := range items {
					if w := lipgloss.Width(items[i].RepoSlug()); w > longest {
						longest = w
					}
				}
				return min(longest, max(minRepoCol, listWidth/repoColDivisor))
			},
			render: func(pr *bitbucket.PullRequest, width int, _ bool) string {
				return padRight(components.TruncateEnd(pr.RepoSlug(), width), width)
			},
		},
		config.FieldID: {
			id: config.FieldID,
			width: func(items []bitbucket.PullRequest, _ int) int {
				longest := 0
				for i := range items {
					if w := len(prIDLabel(&items[i])); w > longest {
						longest = w
					}
				}
				return longest
			},
			render: func(pr *bitbucket.PullRequest, width int, _ bool) string {
				return padRight(prIDLabel(pr), width)
			},
		},
		config.FieldTitle: {
			id:    config.FieldTitle,
			width: func([]bitbucket.PullRequest, int) int { return 0 },
			render: func(pr *bitbucket.PullRequest, width int, _ bool) string {
				title := pr.Title
				if pr.Draft {
					title = "[draft] " + title
				}
				return padRight(components.TruncateEnd(title, width), width)
			},
		},
		config.FieldAuthor: {
			id:    config.FieldAuthor,
			width: func([]bitbucket.PullRequest, int) int { return authorCol },
			render: func(pr *bitbucket.PullRequest, width int, selected bool) string {
				name := components.TruncateEnd(pr.Author.Name(), width)
				if selected {
					return padRight(name, width)
				}
				return padRight(theme.AuthorRender(name), width)
			},
		},
		config.FieldApprovals: {
			id:    config.FieldApprovals,
			width: func([]bitbucket.PullRequest, int) int { return approvalsCol },
			render: func(pr *bitbucket.PullRequest, width int, selected bool) string {
				return padRight(approvalsCell(pr, selected), width)
			},
		},
		config.FieldBuild: {
			id:    config.FieldBuild,
			width: func([]bitbucket.PullRequest, int) int { return buildGlyphCol },
			render: func(pr *bitbucket.PullRequest, width int, selected bool) string {
				return padRight(buildGlyph(m.builds[pr.Key()], selected), width)
			},
		},
		config.FieldUpdated: {
			id:    config.FieldUpdated,
			width: func([]bitbucket.PullRequest, int) int { return updatedCol },
			render: func(pr *bitbucket.PullRequest, width int, _ bool) string {
				return padRight(timeAgo(pr.UpdatedOn), width)
			},
		},
		config.FieldBranch: {
			id:    config.FieldBranch,
			width: func([]bitbucket.PullRequest, int) int { return branchCol },
			render: func(pr *bitbucket.PullRequest, width int, _ bool) string {
				return padRight(components.TruncateEnd(pr.SourceBranch(), width), width)
			},
		},
		config.FieldState: {
			id:    config.FieldState,
			width: func([]bitbucket.PullRequest, int) int { return stateCol },
			render: func(pr *bitbucket.PullRequest, width int, selected bool) string {
				label := components.TruncateEnd(string(pr.State), width)
				if selected {
					return padRight(label, width)
				}
				return padRight(stateStyle(pr.State).Render(label), width)
			},
		},
	}
}

// columnLayout resolves the configured fields into columns sized for listWidth.
// Columns are dropped from the end (never the title) until the title column has
// at least minTitleWidth columns, which is what makes the ~38-column side panel
// readable.
func (m *PullRequestList) columnLayout(listWidth int) []column {
	fields := m.fields
	if len(fields) == 0 {
		fields = config.DefaultPullRequestListFields()
	}
	specs := m.columnSpecs()

	cols := make([]column, 0, len(fields))
	for _, f := range fields {
		spec, ok := specs[f]
		if !ok {
			continue
		}
		cols = append(cols, column{spec: spec, width: spec.width(m.items, listWidth)})
	}
	if len(cols) == 0 {
		return nil
	}

	for {
		fixed, hasTitle := 1, false
		if len(cols) > 1 {
			fixed += len(cols) - 1 // one space between columns
		}
		for _, c := range cols {
			if c.spec.id == config.FieldTitle {
				hasTitle = true
				continue
			}
			fixed += c.width
		}
		if !hasTitle || listWidth-fixed >= minTitleWidth {
			sizeTitleColumn(cols, listWidth-fixed)
			return cols
		}
		last := lastDroppable(cols)
		if last < 0 {
			sizeTitleColumn(cols, listWidth-fixed)
			return cols
		}
		cols = append(cols[:last], cols[last+1:]...)
	}
}

// sizeTitleColumn gives the flexible column whatever room is left, with a floor
// so a very narrow panel still shows a few characters of title.
func sizeTitleColumn(cols []column, remaining int) {
	for i := range cols {
		if cols[i].spec.id == config.FieldTitle {
			cols[i].width = max(remaining, 5)
			return
		}
	}
}

// lastDroppable is the index of the last column that is not the title.
func lastDroppable(cols []column) int {
	for i, c := range slices.Backward(cols) {
		if c.spec.id != config.FieldTitle {
			return i
		}
	}
	return -1
}

func (m *PullRequestList) renderRow(pr *bitbucket.PullRequest, layout []column, width int, selected bool) string {
	parts := make([]string, 0, len(layout))
	for _, c := range layout {
		parts = append(parts, c.spec.render(pr, c.width, selected))
	}
	line := " " + strings.Join(parts, " ")
	if ansi.StringWidth(line) > width {
		line = ansi.Truncate(line, width, "")
	}
	if selected && m.Focused {
		return m.theme.SelectedItem.Width(width).Render(line)
	}
	return m.theme.NormalItem.Width(width).Render(line)
}

func prIDLabel(pr *bitbucket.PullRequest) string {
	return "#" + strconv.Itoa(pr.ID)
}

// approvalsCell renders the review rollup: "✓N" once someone approved and
// nobody blocked, "✗N" when changes were requested, "·" when nobody has acted
// and "?" when the payload carried no review data at all.
func approvalsCell(pr *bitbucket.PullRequest, plain bool) string {
	gray := lipgloss.NewStyle().Foreground(theme.ColorGray)
	if !pr.HasReviewData() {
		if plain {
			return "?"
		}
		return gray.Render("?")
	}
	summary := pr.ReviewSummary()
	switch {
	case summary.ChangesRequested > 0:
		label := "✗" + strconv.Itoa(summary.ChangesRequested)
		if plain {
			return label
		}
		return lipgloss.NewStyle().Foreground(theme.ColorRed).Render(label)
	case summary.Approved > 0:
		label := "✓" + strconv.Itoa(summary.Approved)
		if plain {
			return label
		}
		return lipgloss.NewStyle().Foreground(theme.ColorGreen).Render(label)
	default:
		if plain {
			return "·"
		}
		return gray.Render("·")
	}
}

// buildGlyph is the one-character build column. An unknown state renders as a
// space so the column keeps its width without claiming a result.
func buildGlyph(state bitbucket.CommitStatusState, plain bool) string {
	var (
		glyph string
		color lipgloss.TerminalColor
	)
	switch state {
	case bitbucket.StatusSuccessful:
		glyph, color = "✔", theme.ColorGreen
	case bitbucket.StatusFailed:
		glyph, color = "✘", theme.ColorRed
	case bitbucket.StatusInProgress:
		glyph, color = "●", theme.ColorYellow
	case bitbucket.StatusStopped:
		glyph, color = "■", theme.ColorGray
	default:
		return " "
	}
	if plain {
		return glyph
	}
	return lipgloss.NewStyle().Foreground(color).Render(glyph)
}

// stateStyle colours a pull request state: open blue, merged green, declined
// red, anything else gray.
func stateStyle(state bitbucket.PullRequestState) lipgloss.Style {
	switch state {
	case bitbucket.PullRequestOpen:
		return lipgloss.NewStyle().Foreground(theme.ColorBlue)
	case bitbucket.PullRequestMerged:
		return lipgloss.NewStyle().Foreground(theme.ColorGreen)
	case bitbucket.PullRequestDeclined:
		return lipgloss.NewStyle().Foreground(theme.ColorRed)
	case bitbucket.PullRequestSuperseded:
		return lipgloss.NewStyle().Foreground(theme.ColorGray)
	default:
		return lipgloss.NewStyle().Foreground(theme.ColorGray)
	}
}
