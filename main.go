package main

import (
	"bufio"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- EMBEDDING ---
//
//go:embed themes.json
var embeddedThemesFS embed.FS

// --- ENUMS & CONSTANTS ---

type appState int

const (
	viewMain appState = iota
	viewTrash
	viewThemeSelector
)

const (
	appName           = "todo-app"
	defaultThemesFile = "themes.json"
	configFile        = "config.json"
)

// --- CONFIGURATION ---

type Config struct {
	SelectedTheme string `json:"selected_theme"`
}

// --- THEME SYSTEM ---

type JSONTheme struct {
	Name      string `json:"name"`
	Base      string `json:"base"`
	Highlight string `json:"highlight"`
	Text      string `json:"text"`
	Comment   string `json:"comment"`
	Special   string `json:"special"`
	Error     string `json:"error"`
	Accent    string `json:"accent"`
}

type Theme struct {
	Name      string
	Base      lipgloss.Color
	Highlight lipgloss.Color
	Text      lipgloss.Color
	Comment   lipgloss.Color
	Special   lipgloss.Color
	Error     lipgloss.Color
	Accent    lipgloss.Color
}

var defaultTheme = Theme{
	Name:      "Gruvbox (Built-in)",
	Base:      lipgloss.Color("#282828"),
	Highlight: lipgloss.Color("#fabd2f"),
	Text:      lipgloss.Color("#ebdbb2"),
	Comment:   lipgloss.Color("#928374"),
	Special:   lipgloss.Color("#b8bb26"),
	Error:     lipgloss.Color("#fb4934"),
	Accent:    lipgloss.Color("#83a598"),
}

var themes []Theme

// --- DATA MODEL ---

type item struct {
	title     string
	done      bool
	level     int
	collapsed bool
}

type visibleItem struct {
	index int
	data  item
}

type model struct {
	items    []item
	trash    []item
	filename string

	visibleItems []visibleItem

	state    appState
	quitting bool

	inputMode      bool
	editMode       bool
	addSubtaskMode bool
	inputBuf       string

	cursorMain  int
	cursorTrash int
	cursorTheme int

	width       int
	height      int
	activeTheme Theme
}

// --- INITIALIZATION ---

func initialModel(filename string) model {
	loadedThemes := loadThemes()
	if len(loadedThemes) > 0 {
		themes = loadedThemes
	} else {
		themes = []Theme{defaultTheme}
	}

	config := loadConfig()
	startTheme := themes[0]

	for _, t := range themes {
		if t.Name == config.SelectedTheme {
			startTheme = t
			break
		}
	}

	activeItems, trashItems := loadTodo(filename)

	m := model{
		items:       activeItems,
		trash:       trashItems,
		cursorMain:  0,
		filename:    filename,
		activeTheme: startTheme,
		state:       viewMain,
	}
	m.recalcVisible()

	for i, t := range themes {
		if t.Name == startTheme.Name {
			m.cursorTheme = i
			break
		}
	}

	return m
}

func (m *model) recalcVisible() {
	m.visibleItems = []visibleItem{}
	currentCollapseLevel := -1

	for i, item := range m.items {
		if currentCollapseLevel != -1 {
			if item.level > currentCollapseLevel {
				continue
			} else {
				currentCollapseLevel = -1
			}
		}

		m.visibleItems = append(m.visibleItems, visibleItem{index: i, data: item})

		if item.collapsed {
			currentCollapseLevel = item.level
		}
	}

	if m.cursorMain >= len(m.visibleItems) {
		m.cursorMain = max(0, len(m.visibleItems)-1)
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (m model) Init() tea.Cmd {
	return nil
}

// --- UPDATE LOGIC ---

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		if m.inputMode {
			switch msg.Type {
			case tea.KeyEnter:
				m.handleInputConfirm()

			case tea.KeyEsc:
				m.handleInputCancel()

			case tea.KeyBackspace, tea.KeyDelete:
				if len(m.inputBuf) > 0 {
					m.inputBuf = m.inputBuf[:len(m.inputBuf)-1]
				}
			case tea.KeySpace:
				m.inputBuf += " "
			case tea.KeyRunes:
				m.inputBuf += string(msg.Runes)
			}
			return m, nil
		}

		switch msg.String() {
		case "ctrl+c", "q":
			if m.state != viewMain {
				m.state = viewMain
				return m, nil
			}
			m.quitting = true
			return m, tea.Quit
		}

		switch m.state {
		case viewMain:
			return m.updateMain(msg)
		case viewTrash:
			return m.updateTrash(msg)
		case viewThemeSelector:
			return m.updateThemeSelector(msg)
		}
	}
	return m, nil
}

func (m *model) handleInputConfirm() {
	if len(m.inputBuf) == 0 && !m.editMode {
		m.handleInputCancel()
		return
	}

	realIdx := m.visibleItems[m.cursorMain].index
	m.items[realIdx].title = m.inputBuf

	m.inputMode = false
	m.editMode = false
	m.inputBuf = ""

	// FIX: Odśwież listę widoczną (zaktualizuj kopie danych)
	m.recalcVisible()

	saveTodo(m.filename, m.items, m.trash)
}

func (m *model) handleInputCancel() {
	if m.editMode {
		m.inputMode = false
		m.editMode = false
		m.inputBuf = ""
	} else {
		realIdx := m.visibleItems[m.cursorMain].index
		m.items = append(m.items[:realIdx], m.items[realIdx+1:]...)

		m.recalcVisible()
		if m.cursorMain > 0 {
			m.cursorMain--
		}

		m.inputMode = false
		m.inputBuf = ""
	}
}

func (m model) updateMain(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	realIdx := -1
	if len(m.visibleItems) > 0 {
		realIdx = m.visibleItems[m.cursorMain].index
	}

	switch msg.String() {
	case "up", "k":
		if m.cursorMain > 0 {
			m.cursorMain--
		}
	case "down", "j":
		if m.cursorMain < len(m.visibleItems)-1 {
			m.cursorMain++
		}
	case " ":
		if realIdx != -1 {
			m.items[realIdx].done = !m.items[realIdx].done
			saveTodo(m.filename, m.items, m.trash)
			m.recalcVisible()
		}
	case "v":
		if realIdx != -1 {
			hasChildren := false
			if realIdx+1 < len(m.items) && m.items[realIdx+1].level > m.items[realIdx].level {
				hasChildren = true
			}

			if hasChildren {
				m.items[realIdx].collapsed = !m.items[realIdx].collapsed
				m.recalcVisible()
			}
		}
	case "n":
		m.inputMode = true
		m.editMode = false
		m.inputBuf = ""

		newItem := item{title: "", level: 0}
		m.items = append(m.items, newItem)
		m.recalcVisible()
		m.cursorMain = len(m.visibleItems) - 1

	case "m":
		if realIdx != -1 {
			m.inputMode = true
			m.editMode = false
			m.inputBuf = ""

			parent := &m.items[realIdx]
			parent.collapsed = false

			newItem := item{
				title: "",
				level: parent.level + 1,
			}

			m.items = append(m.items[:realIdx+1], append([]item{newItem}, m.items[realIdx+1:]...)...)
			m.recalcVisible()
			m.cursorMain++
		}

	case "e":
		if realIdx != -1 {
			m.inputMode = true
			m.editMode = true
			m.inputBuf = m.items[realIdx].title
		}

	case "d":
		if realIdx != -1 {
			countToDelete := 1
			currentLevel := m.items[realIdx].level

			for i := realIdx + 1; i < len(m.items); i++ {
				if m.items[i].level > currentLevel {
					countToDelete++
				} else {
					break
				}
			}

			deletedSlice := make([]item, countToDelete)
			copy(deletedSlice, m.items[realIdx:realIdx+countToDelete])
			m.trash = append(m.trash, deletedSlice...)

			m.items = append(m.items[:realIdx], m.items[realIdx+countToDelete:]...)

			m.recalcVisible()
			if m.cursorMain >= len(m.visibleItems) && m.cursorMain > 0 {
				m.cursorMain--
			}

			saveTodo(m.filename, m.items, m.trash)
		}
	case "tab":
		if realIdx != -1 {
			if m.items[realIdx].level == 0 {
				m.items[realIdx].level = 1
			} else {
				m.items[realIdx].level = 0
			}
			m.recalcVisible()
			saveTodo(m.filename, m.items, m.trash)
		}
	case "t":
		m.state = viewThemeSelector
	case "B":
		m.state = viewTrash
		m.cursorTrash = 0
	}
	return m, nil
}

func (m model) updateTrash(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "B":
		m.state = viewMain
	case "up", "k":
		if m.cursorTrash > 0 {
			m.cursorTrash--
		}
	case "down", "j":
		if m.cursorTrash < len(m.trash)-1 {
			m.cursorTrash++
		}
	case "enter":
		if len(m.trash) > 0 {
			restored := m.trash[m.cursorTrash]
			m.items = append(m.items, restored)
			m.trash = append(m.trash[:m.cursorTrash], m.trash[m.cursorTrash+1:]...)
			if m.cursorTrash >= len(m.trash) && m.cursorTrash > 0 {
				m.cursorTrash--
			}
			saveTodo(m.filename, m.items, m.trash)
			m.recalcVisible()
		}
	case "x":
		if len(m.trash) > 0 {
			m.trash = append(m.trash[:m.cursorTrash], m.trash[m.cursorTrash+1:]...)
			if m.cursorTrash >= len(m.trash) && m.cursorTrash > 0 {
				m.cursorTrash--
			}
			saveTodo(m.filename, m.items, m.trash)
		}
	}
	return m, nil
}

func (m model) updateThemeSelector(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.state = viewMain
	case "up", "k":
		if m.cursorTheme > 0 {
			m.cursorTheme--
		}
	case "down", "j":
		if m.cursorTheme < len(themes)-1 {
			m.cursorTheme++
		}
	case "enter":
		m.activeTheme = themes[m.cursorTheme]
		saveConfig(m.activeTheme.Name)
		m.state = viewMain
	}
	return m, nil
}

// --- VIEW LOGIC ---

func (m model) View() string {
	if m.quitting {
		return ""
	}

	t := m.activeTheme
	dimStyle := lipgloss.NewStyle().Foreground(t.Comment)

	// --- HEADER ---
	modeName := "TODO"
	if m.state == viewTrash {
		modeName = "BIN"
	} else if m.state == viewThemeSelector {
		modeName = "THEMES"
	}

	fullPath, err := filepath.Abs(m.filename)
	if err != nil {
		fullPath = m.filename
	}

	headerText := fmt.Sprintf("// %s %s", modeName, fullPath)
	styledHeader := lipgloss.NewStyle().Foreground(t.Base).Background(t.Highlight).Bold(true).Padding(0, 1).Render(headerText)
	headerBlock := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, styledHeader)

	// --- FOOTER ---
	help := ""
	switch m.state {
	case viewMain:
		help = "New(n) • Subtask(m) • Edit(e) • Fold(v) • Del(d) • Bin(Shift+B) • Theme(t) • Quit(q)"
	case viewTrash:
		help = "Restore(Enter) • Purge(x) • Back(Esc)"
	case viewThemeSelector:
		help = "Select(Enter) • Back(Esc)"
	}

	if m.inputMode {
		help = "Enter to Confirm • Esc to Cancel"
	}

	footer := dimStyle.Render(help)
	footerBlock := lipgloss.PlaceHorizontal(m.width, lipgloss.Center, footer)

	// --- CONTENT ---
	availableH := m.height - 4
	if availableH < 5 {
		availableH = 5
	}

	var content string
	switch m.state {
	case viewMain:
		content = m.renderList(availableH, t)
	case viewTrash:
		content = m.renderTrash(availableH, t)
	case viewThemeSelector:
		content = m.renderThemeSelector(availableH, t)
	}

	return lipgloss.JoinVertical(lipgloss.Left, headerBlock, content, footerBlock)
}

func (m model) renderList(height int, t Theme) string {
	var s strings.Builder
	start, end := paginator(m.cursorMain, height, len(m.visibleItems))

	for i := start; i < end; i++ {
		vItem := m.visibleItems[i]
		item := vItem.data
		isCursor := (m.cursorMain == i)

		cursor := "  "
		if isCursor {
			cursor = "│ "
		}

		treePrefix := ""
		if item.level == 0 {
			treePrefix = " "
		} else {
			var sb strings.Builder
			sb.WriteString(" ")
			for l := 1; l < item.level; l++ {
				hasContinuation := false
				for k := i + 1; k < len(m.visibleItems); k++ {
					futureItem := m.visibleItems[k].data
					if futureItem.level < l {
						break
					}
					if futureItem.level == l {
						hasContinuation = true
						break
					}
				}
				if hasContinuation {
					sb.WriteString(" │ ")
				} else {
					sb.WriteString("   ")
				}
			}
			isLastInGroup := true
			for k := i + 1; k < len(m.visibleItems); k++ {
				futureItem := m.visibleItems[k].data
				if futureItem.level < item.level {
					break
				}
				if futureItem.level == item.level {
					isLastInGroup = false
					break
				}
			}
			if isLastInGroup {
				sb.WriteString(" └─")
			} else {
				sb.WriteString(" ├─")
			}
			treePrefix = sb.String()
		}

		checkStr := "[ ]"
		checkStyle := lipgloss.NewStyle().Foreground(t.Special)

		if item.collapsed {
			checkStr = "[+]"
			checkStyle = lipgloss.NewStyle().Foreground(t.Accent)
		} else if item.done {
			checkStr = "[✔]"
			checkStyle = lipgloss.NewStyle().Foreground(t.Special)
		} else {
			checkStr = "[ ]"
			checkStyle = lipgloss.NewStyle().Foreground(t.Text)
		}

		// --- INLINE INPUT RENDERING ---
		var titleRendered string
		if isCursor && m.inputMode {
			inputStyle := lipgloss.NewStyle().Foreground(t.Base).Background(t.Highlight)
			titleRendered = inputStyle.Render(m.inputBuf + "█")
		} else {
			titleStyle := lipgloss.NewStyle().Foreground(t.Text)
			if item.done {
				titleStyle = lipgloss.NewStyle().Foreground(t.Comment).Strikethrough(true)
			}
			titleRendered = titleStyle.Render(item.title)
		}

		row := fmt.Sprintf("%s%s%s %s",
			lipgloss.NewStyle().Foreground(t.Highlight).Render(cursor),
			lipgloss.NewStyle().Foreground(t.Comment).Render(treePrefix),
			checkStyle.Render(checkStr),
			titleRendered,
		)
		s.WriteString(row + "\n")
	}

	return lipgloss.NewStyle().
		Width(m.width - 2).Height(height).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Highlight).
		Render(s.String())
}

func (m model) renderTrash(height int, t Theme) string {
	var s strings.Builder
	if len(m.trash) == 0 {
		s.WriteString(lipgloss.NewStyle().Foreground(t.Comment).Render("\n  (Bin is empty)"))
	}
	start, end := paginator(m.cursorTrash, height, len(m.trash))

	for i := start; i < end; i++ {
		item := m.trash[i]
		cursor := "  "
		if m.cursorTrash == i {
			cursor = "│ "
		}

		treePrefix := ""
		if item.level == 0 {
			treePrefix = " "
		} else {
			var sb strings.Builder
			sb.WriteString(" ")
			for l := 1; l < item.level; l++ {
				hasContinuation := false
				for k := i + 1; k < len(m.trash); k++ {
					futureItem := m.trash[k]
					if futureItem.level < l {
						break
					}
					if futureItem.level == l {
						hasContinuation = true
						break
					}
				}
				if hasContinuation {
					sb.WriteString(" │ ")
				} else {
					sb.WriteString("   ")
				}
			}
			isLastInGroup := true
			for k := i + 1; k < len(m.trash); k++ {
				futureItem := m.trash[k]
				if futureItem.level < item.level {
					break
				}
				if futureItem.level == item.level {
					isLastInGroup = false
					break
				}
			}
			if isLastInGroup {
				sb.WriteString(" └─")
			} else {
				sb.WriteString(" ├─")
			}
			treePrefix = sb.String()
		}

		row := fmt.Sprintf("%s%s[D] %s",
			lipgloss.NewStyle().Foreground(t.Error).Render(cursor),
			lipgloss.NewStyle().Foreground(t.Comment).Render(treePrefix),
			lipgloss.NewStyle().Foreground(t.Comment).Strikethrough(true).Render(item.title),
		)
		s.WriteString(row + "\n")
	}

	return lipgloss.NewStyle().
		Width(m.width - 2).Height(height).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Error).
		Render(s.String())
}

func (m model) renderThemeSelector(height int, t Theme) string {
	var s strings.Builder
	for i, theme := range themes {
		cursor := "  "
		if m.cursorTheme == i {
			cursor = "-> "
		}
		nameStyle := lipgloss.NewStyle().Foreground(t.Text)
		if m.cursorTheme == i {
			nameStyle = nameStyle.Foreground(t.Highlight).Bold(true)
		}
		preview := lipgloss.NewStyle().Foreground(theme.Base).Render("■") + " " + lipgloss.NewStyle().Foreground(theme.Highlight).Render("■") + " " + lipgloss.NewStyle().Foreground(theme.Special).Render("■")
		row := fmt.Sprintf("%s%s  %s", lipgloss.NewStyle().Foreground(t.Highlight).Render(cursor), nameStyle.Render(theme.Name), preview)
		s.WriteString(row + "\n")
	}

	return lipgloss.NewStyle().
		Width(m.width - 2).Height(height).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Highlight).
		Render(s.String())
}

func paginator(cursor, height, total int) (int, int) {
	if total == 0 {
		return 0, 0
	}
	start := 0
	end := height
	if total < height {
		end = total
	}
	if cursor >= height {
		start = cursor - height + 1
		end = cursor + 1
	}
	return start, end
}

// --- IO (LOADER) ---

func loadTodo(filename string) ([]item, []item) {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return []item{}, []item{}
	}
	file, _ := os.Open(filename)
	defer file.Close()

	var active []item
	var trash []item

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "- [") {
			isDone := strings.Contains(line, "- [x]")
			isTrash := strings.Contains(line, "- [D]")

			leadingSpaces := 0
			for _, char := range line {
				if char == ' ' {
					leadingSpaces++
				} else {
					break
				}
			}
			level := leadingSpaces / 2

			parts := strings.SplitN(line, "]", 2)
			if len(parts) > 1 {
				newItem := item{title: strings.TrimSpace(parts[1]), done: isDone, level: level}

				if isTrash {
					trash = append(trash, newItem)
				} else {
					active = append(active, newItem)
				}
			}
		}
	}
	return active, trash
}

func saveTodo(filename string, items []item, trash []item) {
	file, _ := os.Create(filename)
	defer file.Close()
	writer := bufio.NewWriter(file)

	for _, item := range items {
		status := " "
		if item.done {
			status = "x"
		}
		prefix := strings.Repeat("  ", item.level)
		line := fmt.Sprintf("%s- [%s] %s\n", prefix, status, item.title)
		writer.WriteString(line)
	}

	for _, item := range trash {
		prefix := strings.Repeat("  ", item.level)
		line := fmt.Sprintf("%s- [D] %s\n", prefix, item.title)
		writer.WriteString(line)
	}

	writer.Flush()
}

// --- IO (Config & Themes - GLOBAL SUPPORT) ---

func loadThemes() []Theme {
	var content []byte
	var err error

	content, err = os.ReadFile(defaultThemesFile)
	if err == nil {
		return parseThemes(content)
	}

	configDir, err := os.UserConfigDir()
	if err == nil {
		globalPath := filepath.Join(configDir, appName, defaultThemesFile)
		content, err = os.ReadFile(globalPath)
		if err == nil {
			return parseThemes(content)
		}
	}

	content, err = embeddedThemesFS.ReadFile(defaultThemesFile)
	if err == nil {
		return parseThemes(content)
	}

	return nil
}

func parseThemes(content []byte) []Theme {
	var jsonThemes []JSONTheme
	if err := json.Unmarshal(content, &jsonThemes); err != nil {
		return nil
	}
	var result []Theme
	for _, jt := range jsonThemes {
		result = append(result, Theme{
			Name:      jt.Name,
			Base:      lipgloss.Color(jt.Base),
			Highlight: lipgloss.Color(jt.Highlight),
			Text:      lipgloss.Color(jt.Text),
			Comment:   lipgloss.Color(jt.Comment),
			Special:   lipgloss.Color(jt.Special),
			Error:     lipgloss.Color(jt.Error),
			Accent:    lipgloss.Color(jt.Accent),
		})
	}
	return result
}

func loadConfig() Config {
	var cfg Config

	if _, err := os.Stat(configFile); err == nil {
		data, _ := os.ReadFile(configFile)
		json.Unmarshal(data, &cfg)
		return cfg
	}

	configDir, err := os.UserConfigDir()
	if err == nil {
		globalPath := filepath.Join(configDir, appName, configFile)
		if _, err := os.Stat(globalPath); err == nil {
			data, _ := os.ReadFile(globalPath)
			json.Unmarshal(data, &cfg)
			return cfg
		}
	}

	return cfg
}

func saveConfig(themeName string) {
	cfg := Config{SelectedTheme: themeName}
	data, _ := json.MarshalIndent(cfg, "", "  ")

	if _, err := os.Stat(configFile); err == nil {
		os.WriteFile(configFile, data, 0644)
		return
	}

	configDir, err := os.UserConfigDir()
	if err == nil {
		appDir := filepath.Join(configDir, appName)
		os.MkdirAll(appDir, 0755)
		globalPath := filepath.Join(appDir, configFile)
		os.WriteFile(globalPath, data, 0644)
	}
}

func main() {
	filename := "todo.md"
	if len(os.Args) > 1 {
		filename = os.Args[1]
	}
	p := tea.NewProgram(initialModel(filename), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}
}
