package views

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/lsst25/lazybitbucket/pkg/bitbucket"
	"github.com/lsst25/lazybitbucket/pkg/tui/components"
	"github.com/lsst25/lazybitbucket/pkg/tui/theme"
)

// allReposLabel is what the synthetic "every repository" row shows instead of a
// slug, which it does not have.
const allReposLabel = "All repositories"

// unknownCount marks a repository whose open pull request count is not known
// yet, so the row omits the count instead of claiming zero.
const unknownCount = -1

// RepoHoveredMsg is emitted when the repository cursor moves. It is a hover,
// not a selection: the app previews the repository and only filters the pull
// request list when the user actually activates the row.
type RepoHoveredMsg struct {
	Repo *RepoItem
}

// RepoItem is one row of the repository list. The synthetic All row carries an
// empty Slug and All set, and is what clears the repository filter.
type RepoItem struct {
	Slug       string
	Name       string
	FullName   string
	MainBranch string
	URL        string
	OpenCount  int
	All        bool
}

// Label is what the row shows: the slug, or the all-repositories label for the
// synthetic row.
func (r RepoItem) Label() string {
	if r.All || r.Slug == "" {
		return allReposLabel
	}
	return r.Slug
}

// RepoList is the [4] panel: the configured or discovered repositories, with
// the synthetic All row pinned at the top.
type RepoList struct {
	components.ListBase
	repos      []RepoItem
	all        []RepoItem
	filter     string
	activeSlug string
	theme      *theme.Theme
}

// NewRepoList constructs a list holding only the All row.
func NewRepoList() *RepoList {
	list := &RepoList{theme: theme.Default}
	list.SetRepos(nil)
	return list
}

// SetRepos replaces the repositories, prepending the synthetic All row. Counts
// already known are preserved by slug so a repository refresh does not blank
// the numbers.
func (r *RepoList) SetRepos(repos []bitbucket.Repository) {
	prev := make(map[string]int, len(r.all))
	for _, item := range r.all {
		prev[item.Slug] = item.OpenCount
	}

	items := make([]RepoItem, 0, len(repos)+1)
	items = append(items, RepoItem{
		Name:      allReposLabel,
		OpenCount: unknownCount,
		All:       true,
	})
	for _, repo := range repos {
		count, ok := prev[repo.Slug]
		if !ok {
			count = unknownCount
		}
		items = append(items, RepoItem{
			Slug:       repo.Slug,
			Name:       repo.Name,
			FullName:   repo.FullName,
			MainBranch: repo.MainBranchName(),
			URL:        repo.HTMLURL(),
			OpenCount:  count,
		})
	}
	r.all = items
	r.applyFilter()
}

// SetCounts records the open pull request count per repository slug. The All
// row shows their sum; a repository missing from counts becomes unknown again.
func (r *RepoList) SetCounts(counts map[string]int) {
	total := 0
	known := false
	for i := range r.all {
		if r.all[i].All {
			continue
		}
		if count, ok := counts[r.all[i].Slug]; ok {
			r.all[i].OpenCount = count
			total += count
			known = true
		} else {
			r.all[i].OpenCount = unknownCount
		}
	}
	for i := range r.all {
		if r.all[i].All {
			if known {
				r.all[i].OpenCount = total
			} else {
				r.all[i].OpenCount = unknownCount
			}
		}
	}
	r.applyFilter()
}

// SetActiveSlug marks the repository the pull request list is filtered by. An
// empty slug marks the All row.
func (r *RepoList) SetActiveSlug(slug string) {
	r.activeSlug = slug
	r.applyFilter()
}

// ActiveSlug returns the marked repository slug, "" for All.
func (r *RepoList) ActiveSlug() string { return r.activeSlug }

// SetFilter applies the search bar query to slugs and names.
func (r *RepoList) SetFilter(query string) {
	r.filter = query
	r.applyFilter()
}

func (r *RepoList) applyFilter() {
	if r.filter == "" {
		r.repos = r.all
	} else {
		q := strings.ToLower(r.filter)
		filtered := make([]RepoItem, 0, len(r.all))
		for _, item := range r.all {
			haystack := strings.ToLower(item.Label() + " " + item.Name)
			if strings.Contains(haystack, q) {
				filtered = append(filtered, item)
			}
		}
		r.repos = filtered
	}
	r.Cursor = 0
	r.Offset = 0
	r.SetItemCount(len(r.repos))
}

// SelectedRepo returns a copy of the row under the cursor, nil when empty.
func (r *RepoList) SelectedRepo() *RepoItem {
	if r.Cursor < 0 || r.Cursor >= len(r.repos) {
		return nil
	}
	item := r.repos[r.Cursor]
	return &item
}

// ContentHeight is the natural panel height for the accordion layout.
func (r *RepoList) ContentHeight() int { return r.ListBase.ContentHeight(5) }

// Init satisfies the bubbletea component shape; the list has no startup work.
func (r *RepoList) Init() tea.Cmd { return nil }

// Update moves the cursor and announces the hovered repository.
func (r *RepoList) Update(msg tea.Msg) (*RepoList, tea.Cmd) {
	if !r.Focused {
		return r, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		if r.KeyNav(key.String()) {
			if repo := r.SelectedRepo(); repo != nil {
				return r, func() tea.Msg { return RepoHoveredMsg{Repo: repo} }
			}
		}
	}
	return r, nil
}

// View renders the panel, collapsing to a single title bar at height 1.
func (r *RepoList) View() string {
	footer := ""
	if n := len(r.repos); n > 0 {
		footer = fmt.Sprintf("%d of %d", r.Cursor+1, n)
	}

	const title = "[4] Repos"
	if r.Height <= 1 {
		return components.RenderCollapsedBar(title, footer, r.Width, r.Focused)
	}

	contentWidth, innerHeight := components.PanelDimensions(r.Width, r.Height)

	var rows []string
	end := min(r.Offset+innerHeight, len(r.repos))
	for i := r.Offset; i < end; i++ {
		rows = append(rows, r.renderRow(r.repos[i], contentWidth, i == r.Cursor))
	}

	scroll := &components.ScrollInfo{Total: len(r.repos), Visible: innerHeight, Offset: r.Offset}
	return components.RenderPanelFull(title, footer, strings.Join(rows, "\n"),
		r.Width, innerHeight, r.Focused, scroll)
}

func (r *RepoList) renderRow(item RepoItem, width int, selected bool) string {
	active := item.Slug == r.activeSlug
	marker := " "
	if active {
		marker = "*"
	}

	count := ""
	if item.OpenCount >= 0 {
		count = " · " + strconv.Itoa(item.OpenCount) + " open"
	}

	maxLabel := max(width-1-lipgloss.Width(count), 3)
	label := padRight(components.TruncateEnd(item.Label(), maxLabel), maxLabel)
	line := marker + label + count

	switch {
	case selected && r.Focused:
		return r.theme.SelectedItem.Width(width).Render(line)
	case active:
		coloredMarker := lipgloss.NewStyle().Foreground(theme.ColorAccent).Render(marker)
		return r.theme.NormalItem.Width(width).Render(coloredMarker + label + count)
	default:
		return r.theme.NormalItem.Width(width).Render(line)
	}
}
