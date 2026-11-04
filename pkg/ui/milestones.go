package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Dicklesworthstone/beads_viewer/pkg/loader"
	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// MilestoneModel is the focused, hierarchy-first interface. Tracker mutations
// still go through the source's verified br/bd executable.
type MilestoneModel struct {
	tree          TreeModel
	path          string
	width, height int
	input         textinput.Model
	creating      string
	parent        string
	origin        *model.IssueOrigin
	busy          bool
	message       string
	detail        bool
	viewport      viewport.Model
	modified      time.Time
	fileSize      int64
}

type milestoneRefreshMsg struct{}
type milestoneWriteMsg struct {
	err     error
	created bool
}

// NewMilestoneModel opens a title-first tree without starting graph analysis.
func NewMilestoneModel(issues []model.Issue, path string) *MilestoneModel {
	theme := DefaultTheme(lipgloss.NewRenderer(os.Stdout))
	m := &MilestoneModel{tree: NewTreeModel(theme), path: path, width: 100, height: 30, viewport: viewport.New(96, 20)}
	if path != "" {
		m.tree.SetBeadsDir(filepath.Dir(path))
	}
	m.rebuild(issues)
	return m
}

func isMilestone(issue *model.Issue) bool {
	if issue == nil {
		return false
	}
	if issue.IssueType == model.TypeEpic {
		return true
	}
	for _, label := range issue.Labels {
		if label == "milestone" {
			return true
		}
	}
	return false
}

func (m *MilestoneModel) rebuild(issues []model.Issue) {
	selected := m.tree.GetSelectedID()
	expanded := make(map[string]bool)
	for id, node := range m.tree.issueMap {
		expanded[id] = node.Expanded
	}
	m.tree.Build(issues)
	var order func([]*IssueTreeNode)
	order = func(nodes []*IssueTreeNode) {
		sort.SliceStable(nodes, func(i, j int) bool {
			a, b := nodes[i].Issue, nodes[j].Issue
			if isMilestone(a) != isMilestone(b) {
				return isMilestone(a)
			}
			if !a.CreatedAt.Equal(b.CreatedAt) {
				return a.CreatedAt.Before(b.CreatedAt)
			}
			return a.ID < b.ID
		})
		for _, node := range nodes {
			if value, ok := expanded[node.Issue.ID]; ok {
				node.Expanded = value
			}
			order(node.Children)
		}
	}
	order(m.tree.roots)
	m.tree.rebuildFlatList()
	m.tree.SelectByID(selected)
}

func milestoneTick() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return milestoneRefreshMsg{} })
}

// Init starts periodic refresh so changes made by agents appear automatically.
func (m *MilestoneModel) Init() tea.Cmd { return milestoneTick() }

// Update handles tree navigation, on-demand details, and creation forms.
func (m *MilestoneModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = max(msg.Width, 1), max(msg.Height, 1)
		m.viewport.Width, m.viewport.Height = max(m.width-4, 1), max(m.height-7, 1)
		m.input.Width = max(m.width-8, 1)
		if m.detail {
			m.showDetail()
		}
	case milestoneRefreshMsg:
		if !m.busy && m.sourceChanged() {
			m.reload()
		}
		return m, milestoneTick()
	case milestoneWriteMsg:
		m.busy = false
		if msg.created {
			m.creating = ""
			m.reload()
		}
		if msg.err != nil {
			m.message = msg.err.Error()
		} else {
			m.creating = ""
			m.message = "Created."
			m.reload()
		}
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.busy {
			return m, nil
		}
		if m.creating != "" {
			switch msg.String() {
			case "esc":
				m.creating = ""
				m.message = ""
			case "enter":
				if strings.TrimSpace(m.input.Value()) != "" {
					m.busy = true
					return m, m.createCommand()
				}
			default:
				var cmd tea.Cmd
				m.input, cmd = m.input.Update(msg)
				return m, cmd
			}
			return m, nil
		}
		if m.detail {
			switch msg.String() {
			case "esc", "enter", "tab":
				m.detail = false
			case "q":
				return m, tea.Quit
			default:
				var cmd tea.Cmd
				m.viewport, cmd = m.viewport.Update(msg)
				return m, cmd
			}
			return m, nil
		}
		m.tree.SetSize(max(m.width-4, 1), max(m.height-7, 1))
		switch msg.String() {
		case "q":
			return m, tea.Quit
		case "j", "down":
			m.tree.MoveDown()
		case "k", "up":
			m.tree.MoveUp()
		case "h", "left":
			m.tree.CollapseOrJumpToParent()
		case "l", "right":
			m.tree.ExpandOrMoveToChild()
		case " ":
			m.tree.ToggleExpand()
		case "o":
			m.tree.ExpandAll()
		case "O":
			m.tree.CollapseAll()
		case "g", "home":
			m.tree.JumpToTop()
		case "G", "end":
			m.tree.JumpToBottom()
		case "pgdown", "ctrl+d":
			m.tree.PageDown()
		case "pgup", "ctrl+u":
			m.tree.PageUp()
		case "r", "ctrl+r":
			m.reload()
		case "enter", "tab":
			if m.tree.SelectedIssue() != nil {
				m.detail = true
				m.showDetail()
				m.viewport.GotoTop()
			}
		case "m":
			return m, m.beginCreate("milestone", false)
		case "M":
			return m, m.beginCreate("milestone", true)
		case "n":
			return m, m.beginCreate("issue", true)
		}
	default:
		if m.creating != "" && !m.busy {
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

func (m *MilestoneModel) reload() {
	if m.path == "" {
		return
	}
	// Capture the version before parsing. A write during the read must still
	// trigger another refresh rather than caching its newer timestamp.
	info, statErr := os.Stat(m.path)
	loaded, err := loadIssuesForReload(m.path, loader.ParseOptions{})
	if err != nil {
		m.message = "Refresh failed: " + err.Error()
		return
	}
	// Keep value copies, never pointers into the loader's recyclable pool.
	issues := make([]model.Issue, len(loaded.Issues))
	for i := range loaded.Issues {
		issues[i] = loaded.Issues[i].Clone()
	}
	loader.ReturnIssuePtrsToPool(loaded.PoolRefs)
	m.rebuild(issues)
	if statErr == nil {
		m.modified, m.fileSize = info.ModTime(), info.Size()
	}
	if m.detail {
		m.showDetail()
	}
}

func (m *MilestoneModel) sourceChanged() bool {
	// SQLite writes may live in a WAL, so only skip reads for unchanged JSONL.
	if filepath.Ext(m.path) != ".jsonl" {
		return true
	}
	info, err := os.Stat(m.path)
	return err != nil || !info.ModTime().Equal(m.modified) || info.Size() != m.fileSize
}

func (m *MilestoneModel) beginCreate(kind string, child bool) tea.Cmd {
	issue := m.tree.SelectedIssue()
	if issue == nil && len(m.tree.roots) == 0 {
		// Resolve the source even when its tracker currently contains no issues.
		placeholder := []model.Issue{{ID: "new-milestone"}}
		loader.AttachIssueOrigins(placeholder, m.path, true)
		issue = &placeholder[0]
	}
	if issue == nil {
		return nil
	}
	if err := editorOriginError(issue.Origin); err != nil {
		m.message = err.Error()
		return nil
	}
	origin := *issue.Origin
	m.origin, m.parent = &origin, ""
	if child && issue.ID != "new-milestone" {
		m.parent = origin.LocalID
	}
	m.creating, m.message = kind, ""
	m.input = textinput.New()
	m.input.Placeholder = "Title"
	m.input.CharLimit = 500
	m.input.Width = max(m.width-8, 1)
	return m.input.Focus()
}

func milestoneCreateArgs(kind, title, parent string) []string {
	args := []string{"create", "--title", title, "--type", "task", "--json"}
	if kind == "milestone" {
		args[4] = "epic"
		args = append(args, "--labels", "milestone")
	}
	if parent != "" {
		args = append(args, "--parent", parent)
	}
	return args
}

func (m *MilestoneModel) createCommand() tea.Cmd {
	origin, path := *m.origin, m.path
	args := milestoneCreateArgs(m.creating, strings.TrimSpace(m.input.Value()), m.parent)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), brUpdateTimeout)
		defer cancel()
		if err := loader.RevalidateIssueOrigin(path, origin); err != nil {
			return milestoneWriteMsg{err: err}
		}
		_, diagnostic, err := runEditorTracker(ctx, origin, true, args...)
		if err != nil {
			return milestoneWriteMsg{err: fmt.Errorf("create failed: %w %s", err, diagnostic)}
		}
		if err := refreshEditorExport(ctx, origin, path); err != nil {
			// The write succeeded. Close the form to prevent duplicate creation.
			return milestoneWriteMsg{created: true, err: fmt.Errorf("created, but export refresh failed; press r to refresh: %w", err)}
		}
		return milestoneWriteMsg{created: true}
	}
}

func (m *MilestoneModel) showDetail() {
	issue := m.tree.SelectedIssue()
	if issue == nil {
		m.detail = false
		return
	}
	text := fmt.Sprintf("%s\n%s  ·  %s  ·  P%d\n\n%s", issue.Title, issue.Status, issue.ID, issue.Priority, issue.Description)
	if len(issue.Labels) > 0 {
		text += "\n\nLabels: " + strings.Join(issue.Labels, ", ")
	}
	for _, comment := range issue.Comments {
		if comment != nil {
			text += fmt.Sprintf("\n\n%s\n%s", comment.Author, comment.Text)
		}
	}
	m.viewport.SetContent(ansi.Wrap(text, max(m.width-4, 1), " "))
}

// milestoneProgress counts unique descendant leaves: nested milestone containers
// must not inflate the denominator, and closed branches still count as work.
func milestoneProgress(node *IssueTreeNode) (done, total int) {
	seen := make(map[string]bool)
	var walk func(*IssueTreeNode)
	walk = func(n *IssueTreeNode) {
		if n == nil || n.Issue == nil || seen[n.Issue.ID] {
			return
		}
		seen[n.Issue.ID] = true
		if len(n.Children) == 0 && !isMilestone(n.Issue) {
			total++
			if isClosedLikeStatus(n.Issue.Status) {
				done++
			}
		}
		for _, child := range n.Children {
			walk(child)
		}
	}
	for _, child := range node.Children {
		walk(child)
	}
	return
}

func (m *MilestoneModel) renderRow(node *IssueTreeNode, selected bool, width int) string {
	theme := m.tree.theme
	status := strings.ToUpper(strings.ReplaceAll(string(node.Issue.Status), "_", " "))
	if status == "" {
		status = "OPEN"
	}
	status = ansi.Truncate(status, 11, "…")
	status = theme.Renderer.NewStyle().Foreground(theme.GetStatusColor(string(node.Issue.Status))).Width(11).Render(status)
	prefix := m.tree.buildTreePrefix(node) + m.tree.getExpandIndicator(node) + " "
	title := strings.Join(strings.Fields(node.Issue.Title), " ")
	suffix := ""
	if isMilestone(node.Issue) {
		done, total := milestoneProgress(node)
		suffix = fmt.Sprintf("  %d/%d done", done, total)
		if total == 0 {
			suffix = "  milestone"
		}
	}
	room := max(width-13-lipgloss.Width(prefix), 0)
	if room < lipgloss.Width(suffix)+12 {
		suffix = ""
	}
	title = ansi.Truncate(title, max(room-lipgloss.Width(suffix), 0), "…")
	if isMilestone(node.Issue) {
		title = theme.Renderer.NewStyle().Bold(true).Render(title)
	}
	row := status + "  " + prefix + title + theme.Renderer.NewStyle().Foreground(theme.Muted).Render(suffix)
	row = ansi.Truncate(row, width, "…")
	if selected {
		row = theme.Renderer.NewStyle().Background(ColorBgHighlight).Width(width).Render(row)
	}
	return row
}

// View keeps status at a fixed left edge and gives titles the terminal width.
func (m *MilestoneModel) View() string {
	w, h := max(m.width-4, 1), max(m.height-7, 1)
	m.tree.SetSize(w, h)
	theme := m.tree.theme
	title := "Milestones"
	if m.detail {
		title = "Issue details"
	}
	var body strings.Builder
	body.WriteString(theme.Renderer.NewStyle().Foreground(theme.Primary).Bold(true).Render(title))
	body.WriteString("\n\n")
	if m.creating != "" {
		label := "New " + m.creating
		if m.parent != "" {
			label += " under " + m.parent
		}
		body.WriteString(ansi.Truncate(label, w, "…") + "\n\n" + m.input.View() + "\n\nEnter create · Esc cancel")
	} else if m.detail {
		body.WriteString(m.viewport.View())
	} else if len(m.tree.flatList) == 0 {
		body.WriteString("No issues yet. Press m to create a milestone.")
	} else {
		body.WriteString(theme.Renderer.NewStyle().Foreground(theme.Muted).Render("STATUS       MILESTONE / ISSUE") + "\n")
		start, end := m.tree.visibleRange()
		for i := start; i < end; i++ {
			body.WriteString(m.renderRow(m.tree.flatList[i], i == m.tree.cursor, w))
			body.WriteByte('\n')
		}
	}
	content := lipgloss.NewStyle().Height(max(m.height-4, 1)).MaxHeight(max(m.height-4, 1)).Render(body.String())
	message := m.message
	if message == "" && m.tree.SelectedIssue() != nil {
		message = fmt.Sprintf("%d/%d visible rows · %s", m.tree.cursor+1, len(m.tree.flatList), m.tree.GetSelectedID())
	}
	if m.busy {
		message = "Creating…"
	}
	help := "↑↓ move · ←→ fold · Enter details · m milestone · M submilestone · n child issue · o/O all · q quit"
	if m.detail {
		help = "↑↓ scroll · Esc back · q quit"
	}
	footer := ansi.Truncate(message, w, "…") + "\n" + theme.Renderer.NewStyle().Foreground(theme.Muted).Render(ansi.Truncate(help, w, "…"))
	return lipgloss.NewStyle().Padding(1, 2).MaxWidth(m.width).MaxHeight(m.height).Render(content + "\n" + footer)
}
