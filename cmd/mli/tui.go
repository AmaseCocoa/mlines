package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type tuiMode int

const (
	tuiNav tuiMode = iota
	tuiFilter
	tuiAlias
)

type itemsMsg struct {
	items []ScriptItem
	err   error
}

type doneMsg struct {
	status string
	isErr  bool
	keepID string
}

type tuiModel struct {
	bin       string
	items     []ScriptItem
	filtered  []int
	cursor    int
	offset    int
	input     textinput.Model
	mode      tuiMode
	aliasFor  *ScriptItem
	status    string
	statusErr bool
	showHelp  bool
	width     int
	height    int
}

var (
	tuiTitle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	tuiOn      = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	tuiOff     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	tuiSel     = lipgloss.NewStyle().Background(lipgloss.Color("236")).Bold(true)
	tuiRepo    = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	tuiDesc    = lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
	tuiBadgeSh = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	tuiBadgeBi = lipgloss.NewStyle().Foreground(lipgloss.Color("13"))
	tuiErr     = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	tuiHelp    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	tuiBar     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

func newTUIModel(bin, presetFilter string) tuiModel {
	ti := textinput.New()
	ti.Prompt = "/"
	ti.Placeholder = "filter..."
	ti.CharLimit = 64
	if presetFilter != "" {
		ti.SetValue(presetFilter)
	}
	m := tuiModel{bin: bin, input: ti, cursor: 0}
	if presetFilter != "" {
		m.mode = tuiNav
	} else {
		m.mode = tuiNav
	}
	return m
}

func loadItemsCmd(bin string) tea.Cmd {
	return func() tea.Msg {
		items, err := collectScripts(bin)
		return itemsMsg{items: items, err: err}
	}
}

func (m tuiModel) Init() tea.Cmd {
	return loadItemsCmd(m.bin)
}

func (m *tuiModel) applyFilter() {
	q := strings.ToLower(strings.TrimSpace(m.input.Value()))
	m.filtered = m.filtered[:0]
	for i, it := range m.items {
		if q == "" ||
			strings.Contains(strings.ToLower(it.ID), q) ||
			strings.Contains(strings.ToLower(it.Description), q) {
			m.filtered = append(m.filtered, i)
		}
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *tuiModel) selected() *ScriptItem {
	if len(m.filtered) == 0 || m.cursor < 0 || m.cursor >= len(m.filtered) {
		return nil
	}
	return &m.items[m.filtered[m.cursor]]
}

func (m *tuiModel) setStatus(s string, isErr bool) {
	m.status = s
	m.statusErr = isErr
}

func toggleCmd(bin string, it ScriptItem) tea.Cmd {
	return func() tea.Msg {
		if it.Enabled {
			alias, err := disableScript(bin, it.EnabledAs, false)
			if err != nil {
				return doneMsg{status: err.Error(), isErr: true, keepID: it.ID}
			}
			return doneMsg{status: fmt.Sprintf("無効化: %s (was %s)", it.ID, alias), keepID: it.ID}
		}
		dst, err := enableScript(bin, it.Repo, it.Name, it.Name, false)
		if err != nil {
			hint := err.Error()
			if strings.Contains(hint, "既に存在") || strings.Contains(hint, "有効です") {
				hint += " / 別名で有効化は a キー"
			}
			return doneMsg{status: hint, isErr: true, keepID: it.ID}
		}
		return doneMsg{status: fmt.Sprintf("有効化: %s -> %s", it.ID, dst), keepID: it.ID}
	}
}

func enableAliasCmd(bin string, it ScriptItem, alias string) tea.Cmd {
	return func() tea.Msg {
		dst, err := enableScript(bin, it.Repo, it.Name, alias, false)
		if err != nil {
			return doneMsg{status: err.Error(), isErr: true, keepID: it.ID}
		}
		return doneMsg{status: fmt.Sprintf("有効化: %s として %s", alias, dst), keepID: it.ID}
	}
}

func syncCmd(bin, repo string) tea.Cmd {
	return func() tea.Msg {
		if repo == "" {
			results := syncAllRepos()
			ok, ng := 0, 0
			var msgs []string
			// 決定的な順序で
			repos, _ := listRepos()
			for _, r := range repos {
				if err := results[r.Name]; err != nil {
					ng++
					msgs = append(msgs, r.Name+": 失敗")
				} else {
					ok++
				}
			}
			s := fmt.Sprintf("sync完了: %d件成功 %d件失敗", ok, ng)
			if len(msgs) > 0 {
				s += " (" + strings.Join(msgs, ", ") + ")"
			}
			return doneMsg{status: s, isErr: ng > 0}
		}
		out, err := syncRepo(repo)
		if err != nil {
			return doneMsg{status: fmt.Sprintf("%s: %v", repo, err), isErr: true}
		}
		first := strings.SplitN(out, "\n", 2)[0]
		if first == "" {
			first = "Already up to date."
		}
		return doneMsg{status: fmt.Sprintf("%s: %s", repo, first)}
	}
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case itemsMsg:
		if msg.err != nil {
			m.setStatus(msg.err.Error(), true)
			return m, nil
		}
		keep := ""
		if sel := m.selected(); sel != nil {
			keep = sel.ID
		}
		m.items = msg.items
		m.applyFilter()
		if keep != "" {
			for i, idx := range m.filtered {
				if m.items[idx].ID == keep {
					m.cursor = i
					break
				}
			}
		}
		m.ensureVisible()
		return m, nil
	case doneMsg:
		m.setStatus(msg.status, msg.isErr)
		return m, loadItemsCmd(m.bin)
	}

	switch m.mode {
	case tuiFilter:
		return m.updateFilter(msg)
	case tuiAlias:
		return m.updateAlias(msg)
	default:
		return m.updateNav(msg)
	}
}

func (m tuiModel) updateNav(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.ensureVisible()
			}
		case "down", "j":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
				m.ensureVisible()
			}
		case "g":
			m.cursor = 0
			m.offset = 0
		case "G":
			m.cursor = max(0, len(m.filtered)-1)
			m.ensureVisible()
		case " ", "x", "enter":
			if sel := m.selected(); sel != nil {
				m.setStatus("切り替え中...", false)
				return m, toggleCmd(m.bin, *sel)
			}
		case "/":
			m.mode = tuiFilter
			m.input.Prompt = "/"
			m.input.Placeholder = "filter..."
			m.input.Focus()
			return m, textinput.Blink
		case "a":
			if sel := m.selected(); sel != nil {
				if sel.Enabled {
					m.setStatus(sel.ID+" は既に有効です (無効化は space)", true)
					return m, nil
				}
				cp := *sel
				m.aliasFor = &cp
				m.mode = tuiAlias
				m.input.SetValue(sel.Name)
				m.input.Prompt = "alias: "
				m.input.Focus()
				m.input.CursorEnd()
				return m, textinput.Blink
			}
		case "r":
			if sel := m.selected(); sel != nil {
				m.setStatus(sel.Repo+" をsync中...", false)
				return m, syncCmd(m.bin, sel.Repo)
			}
		case "R":
			m.setStatus("全リポジトリをsync中...", false)
			return m, syncCmd(m.bin, "")
		case "?", "h":
			m.showHelp = !m.showHelp
		case "esc":
			if m.input.Value() != "" {
				m.input.SetValue("")
				m.applyFilter()
			}
		}
	}
	return m, nil
}

func (m tuiModel) updateFilter(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.input.SetValue("")
			m.input.Blur()
			m.mode = tuiNav
			m.applyFilter()
			return m, nil
		case "enter":
			m.input.Blur()
			m.mode = tuiNav
			m.cursor = 0
			m.offset = 0
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.applyFilter()
	return m, cmd
}

func (m tuiModel) updateAlias(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "ctrl+c":
			m.input.Blur()
			m.mode = tuiNav
			m.aliasFor = nil
			m.setStatus("キャンセルしました", false)
			return m, nil
		case "enter":
			alias := strings.TrimSpace(m.input.Value())
			target := m.aliasFor
			m.input.Blur()
			m.mode = tuiNav
			m.aliasFor = nil
			if target == nil || alias == "" {
				m.setStatus("キャンセルしました", false)
				return m, nil
			}
			m.setStatus("有効化中...", false)
			return m, enableAliasCmd(m.bin, *target, alias)
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *tuiModel) ensureVisible() {
	vis := m.visibleRows()
	if vis <= 0 {
		return
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+vis {
		m.offset = m.cursor - vis + 1
	}
}

func (m tuiModel) visibleRows() int {
	// header(3) + footer(2) + 余白
	h := m.height - 6
	if m.showHelp {
		h -= 7
	}
	if m.mode != tuiNav {
		h -= 1
	}
	if h < 3 {
		h = 3
	}
	return h
}

func (m tuiModel) View() string {
	var b strings.Builder
	on, total := 0, len(m.items)
	for _, it := range m.items {
		if it.Enabled {
			on++
		}
	}
	b.WriteString(tuiTitle.Render("mlines") + tuiBar.Render(fmt.Sprintf("  %d/%d 有効  bin: %s", on, total, m.bin)) + "\n")
	if q := m.input.Value(); m.mode == tuiFilter || q != "" {
		b.WriteString(tuiBar.Render("filter: "+q) + "\n")
	} else {
		b.WriteString("\n")
	}

	if len(m.filtered) == 0 {
		b.WriteString(tuiHelp.Render("  該当なし (リポジトリ未登録なら `mli repo add` で追加)") + "\n")
	} else {
		end := min(m.offset+m.visibleRows(), len(m.filtered))
		for i := m.offset; i < end; i++ {
			it := m.items[m.filtered[i]]
			mark := tuiOff.Render("○")
			if it.Enabled {
				mark = tuiOn.Render("●")
			}
			badge := tuiBadgeSh.Render("[sh]")
			if it.Type == "binary" {
				badge = tuiBadgeBi.Render("[bin]")
			}
			as := ""
			if it.Enabled {
				as = tuiHelp.Render(" (" + it.EnabledAs + ")")
			} else if it.Conflict != "" {
				as = tuiErr.Render(" [! " + it.Conflict + "]")
			}
			desc := it.Description
			if desc == "" {
				desc = "-"
			}
			line := fmt.Sprintf(" %s %s %s %s%s",
				mark, tuiRepo.Render(it.ID), badge, tuiDesc.Render(desc), as)
			if i == m.cursor {
				line = tuiSel.Render(line)
			}
			b.WriteString(line + "\n")
		}
	}

	if m.mode == tuiAlias {
		b.WriteString("\n" + m.input.View() + tuiHelp.Render("  (enterで有効化 escで取消)") + "\n")
	} else if m.mode == tuiFilter {
		b.WriteString("\n" + m.input.View() + tuiHelp.Render("  (enterで確定 escでクリア)") + "\n")
	} else {
		b.WriteString("\n")
	}

	if m.showHelp {
		b.WriteString(tuiHelp.Render("  ↑↓/jk 移動  space 有効/無効切替  / 絞り込み  a 別名で有効化\n") +
			tuiHelp.Render("  r このrepoをsync  R 全repoをsync  ? ヘルプ  q 終了") + "\n")
	} else {
		b.WriteString(tuiBar.Render("  space:切替 /:絞込 a:別名 r/R:sync ?:ヘルプ q:終了") + "\n")
	}
	if m.status != "" {
		if m.statusErr {
			b.WriteString(tuiErr.Render("! "+m.status) + "\n")
		} else {
			b.WriteString(tuiBar.Render(m.status) + "\n")
		}
	}
	return b.String()
}

// runTUI は対話TUIを起動する。非TTYでは一覧表示にフォールバックする。
func runTUI(bin, presetFilter string) error {
	if !isTerminal() {
		return printList(bin, false)
	}
	m := newTUIModel(bin, presetFilter)
	if presetFilter != "" {
		m.input.SetValue(presetFilter)
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUIエラー: %w", err)
	}
	return nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
