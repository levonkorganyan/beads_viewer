package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Dicklesworthstone/beads_viewer/pkg/loader"
	"github.com/Dicklesworthstone/beads_viewer/pkg/model"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func milestoneFixture() []model.Issue {
	return []model.Issue{
		{ID: "a-very-long-milestone-id-that-should-not-consume-the-title-column", Title: "M1: Ship the release", IssueType: model.TypeEpic, Status: model.StatusOpen},
		{ID: "child", Title: "Fix 中文界面 and ship 🚀", IssueType: model.TypeTask, Status: model.StatusInProgress,
			Dependencies: []*model.Dependency{{Type: model.DepParentChild, DependsOnID: "a-very-long-milestone-id-that-should-not-consume-the-title-column"}}},
		{ID: "done", Title: "Finished task", IssueType: model.TypeTask, Status: model.StatusClosed,
			Dependencies: []*model.Dependency{{Type: model.DepParentChild, DependsOnID: "a-very-long-milestone-id-that-should-not-consume-the-title-column"}}},
	}
}

func TestMilestoneRowsFitAndKeepStatusAligned(t *testing.T) {
	m := NewMilestoneModel(milestoneFixture(), "")
	for _, width := range []int{1, 12, 30, 60, 100} {
		for _, node := range m.tree.flatList {
			for _, selected := range []bool{false, true} {
				row := m.renderRow(node, selected, width)
				if got := lipgloss.Width(row); got > width {
					t.Fatalf("width %d: row has %d cells: %q", width, got, row)
				}
				if width >= 30 && !strings.HasPrefix(ansi.Strip(row), strings.ToUpper(strings.ReplaceAll(string(node.Issue.Status), "_", " "))) {
					t.Fatalf("status must start at the same column: %q", row)
				}
			}
		}
	}
	row := ansi.Strip(m.renderRow(m.tree.roots[0], false, 60))
	if !strings.Contains(row, "M1: Ship the release") || !strings.Contains(row, "1/2 done") {
		t.Fatal(row)
	}
	if strings.Contains(row, m.tree.roots[0].Issue.ID) {
		t.Fatal("long IDs should be in details, not the tree")
	}
}

func TestMilestoneProgressIgnoresContainersAndDuplicateLeaves(t *testing.T) {
	leaf := &IssueTreeNode{Issue: &model.Issue{ID: "done", IssueType: model.TypeTask, Status: model.StatusClosed}}
	nested := &IssueTreeNode{Issue: &model.Issue{ID: "nested", IssueType: model.TypeEpic}, Children: []*IssueTreeNode{leaf}}
	root := &IssueTreeNode{Issue: &model.Issue{ID: "root", IssueType: model.TypeEpic}, Children: []*IssueTreeNode{nested, leaf, {Issue: &model.Issue{ID: "empty", IssueType: model.TypeEpic}}}}
	if done, total := milestoneProgress(root); done != 1 || total != 1 {
		t.Fatalf("progress = %d/%d", done, total)
	}
}

func TestMilestoneRefreshPreservesSelectionAndFolds(t *testing.T) {
	m := NewMilestoneModel(milestoneFixture(), "")
	m.tree.SetBeadsDir(t.TempDir())
	m.tree.CollapseAll()
	m.rebuild(milestoneFixture())
	if len(m.tree.flatList) != 1 {
		t.Fatal("refresh unfolded tree")
	}
	m.tree.ExpandAll()
	m.tree.SelectByID("child")
	m.rebuild(milestoneFixture())
	if m.tree.GetSelectedID() != "child" {
		t.Fatal("refresh moved selection")
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.detail || !strings.Contains(m.viewport.View(), "Fix 中文界面") {
		t.Fatal("Enter should open selected details")
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.detail || m.tree.GetSelectedID() != "child" {
		t.Fatal("Esc should return to the same tree selection")
	}
}

func TestMilestoneExportFailureDoesNotLeaveDuplicateCreationForm(t *testing.T) {
	m := NewMilestoneModel(nil, "")
	m.creating, m.busy = "milestone", true
	_, _ = m.Update(milestoneWriteMsg{created: true, err: os.ErrPermission})
	if m.creating != "" || m.busy || m.message == "" {
		t.Fatal("successful create with failed export must close the form and show the error")
	}
}

func TestMilestoneRefreshDetectsExternalWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "issues.jsonl")
	if err := os.WriteFile(path, []byte("{\"id\":\"a\",\"title\":\"Before\",\"status\":\"open\",\"issue_type\":\"task\"}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	m := NewMilestoneModel(nil, path)
	m.reload()
	if m.sourceChanged() {
		t.Fatal("unchanged JSONL should not be reparsed every tick")
	}
	if err := os.WriteFile(path, []byte("{\"id\":\"a\",\"title\":\"Changed by agent\",\"status\":\"in_progress\",\"issue_type\":\"task\"}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if !m.sourceChanged() {
		t.Fatal("agent write was not detected")
	}
	_, _ = m.Update(milestoneRefreshMsg{})
	if issue := m.tree.SelectedIssue(); issue == nil || issue.Title != "Changed by agent" || issue.Status != model.StatusInProgress {
		t.Fatalf("refresh did not update issue: %+v", issue)
	}
}

func TestMilestoneCreateInRealTracker(t *testing.T) {
	tracker, err := exec.LookPath("br")
	if err != nil {
		t.Skip("br is not installed")
	}
	dir := t.TempDir()
	cmd := exec.Command(tracker, "init", "--prefix", "test", "--json")
	cmd.Dir = dir
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.HasPrefix(key, "BEADS_") && key != "BD_DB" {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("init: %v: %s", err, output)
	}
	path := filepath.Join(dir, ".beads", "issues.jsonl")
	m := NewMilestoneModel(nil, path)
	m.beginCreate("milestone", false)
	if m.creating == "" {
		t.Fatalf("empty tracker creation: %s", m.message)
	}
	m.input.SetValue("M1: title with $(literal) and 'quotes'")
	result := m.createCommand()().(milestoneWriteMsg)
	if result.err != nil {
		t.Fatal(result.err)
	}
	_, _ = m.Update(result)
	if len(m.tree.roots) != 1 || !isMilestone(m.tree.roots[0].Issue) {
		t.Fatal("milestone was not reloaded")
	}
	parent := m.tree.GetSelectedID()
	m.beginCreate("issue", true)
	m.input.SetValue("Child task")
	result = m.createCommand()().(milestoneWriteMsg)
	if result.err != nil {
		t.Fatal(result.err)
	}
	_, _ = m.Update(result)
	issues, err := loader.LoadIssuesFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 2 || len(m.tree.roots) != 1 || len(m.tree.roots[0].Children) != 1 {
		t.Fatal("child was not nested under its milestone")
	}
	if m.tree.roots[0].Issue.ID != parent {
		t.Fatal("parent changed")
	}
	// An imported source can be browsed but must never borrow a live tracker.
	m.tree.roots[0].Issue.Origin = nil
	m.creating = ""
	m.beginCreate("milestone", false)
	if m.creating != "" {
		t.Fatal("unverified source allowed creation")
	}
}
