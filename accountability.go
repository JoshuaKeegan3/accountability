package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var docStyle = lipgloss.NewStyle().Margin(1, 2)

type item struct {
	title   string
	marked  bool
	hidden  bool
	command string
}

func (i item) Title() string       { return i.title }
func (i item) FilterValue() string { return i.title }
func (i item) Description() string { return "" }

type customDelegate struct {
	list.DefaultDelegate
}

func (d customDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(item)
	if !ok {
		return
	}

	str := fmt.Sprintf("%s", i.title)

	fn := d.Styles.NormalTitle.Render
	if i.marked {
		str = fmt.Sprintf("✅ %s", str)
	}
	if index == m.Index() {
		fn = func(s ...string) string {
			return d.Styles.SelectedTitle.Render(strings.Join(s, " "))
		}
	}

	fmt.Fprint(w, fn(str))
}

type model struct {
	// yesterday list.Model
	// todos     list.Model
	// weekly    list.Model
	yesterday         list.Model
	todos             list.Model
	weekly            list.Model
	focused           int
	configDir         string
	hideCompleted     bool
	allItemsYesterday []list.Item
	allItemsToday     []list.Item
	ShowHelp          bool
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "tab":
			m.focused = (m.focused + 1) % 3
			return m, nil
		case "h", "left":
			m.focused = (m.focused - 1 + 3) % 3
			return m, nil
		case "l", "right":
			m.focused = (m.focused + 1) % 3
			return m, nil
		case "o":
			var i item
			var ok bool
			if m.focused == 0 {
				i, ok = m.yesterday.SelectedItem().(item)
			} else {
				i, ok = m.todos.SelectedItem().(item)
			}
			if !ok {
				return m, nil
			}
			if i.command != "" {
				cmd := exec.Command("sh", "-c", i.command)
				cmd.Start()
			}
			return m, nil
		case "?":
			m.ShowHelp = !m.ShowHelp
			return m, nil
		case "H":
			m.hideCompleted = !m.hideCompleted
			m.yesterday.SetItems(filterCompleted(m.hideCompleted, m.allItemsYesterday))
			m.todos.SetItems(filterCompleted(m.hideCompleted, m.allItemsToday))
			return m, nil
		case " ":
			var i item
			var ok bool
			if m.focused == 0 {
				i, ok = m.yesterday.SelectedItem().(item)
				if !ok {
					return m, nil
				}
				i.marked = !i.marked
				m.yesterday.SetItem(m.yesterday.Index(), i)
				for j, oldItem := range m.allItemsYesterday {
					if oldItem.(item).title == i.title {
						m.allItemsYesterday[j] = i
						break
					}
				}
			} else if m.focused == 1 {
				i, ok = m.todos.SelectedItem().(item)
				if !ok {
					return m, nil
				}
				i.marked = !i.marked
				m.todos.SetItem(m.todos.Index(), i)
				for j, oldItem := range m.allItemsToday {
					if oldItem.(item).title == i.title {
						m.allItemsToday[j] = i
						break
					}
				}
			} else {
				i, ok = m.weekly.SelectedItem().(item)
				if !ok {
					return m, nil
				}
				i.marked = !i.marked
				m.weekly.SetItem(m.weekly.Index(), i)
			}
			return m, nil
		}
	case tea.WindowSizeMsg:
		h, v := docStyle.GetFrameSize()
		m.yesterday.SetSize(msg.Width/3-h, msg.Height-v)
		m.todos.SetSize(msg.Width/3-h, msg.Height-v)
		m.weekly.SetSize(msg.Width/3-h, msg.Height-v)
	}

	switch m.focused {
	case 0:
		m.yesterday, cmd = m.yesterday.Update(msg)
	case 1:
		m.todos, cmd = m.todos.Update(msg)
	default:
		m.weekly, cmd = m.weekly.Update(msg)
	}
	return m, cmd
}

func (m model) View() string {
	if m.ShowHelp {
		return "q: quit\no: open\nspace: mark as done\nH: hide completed\n?: show/hide this help"
	}
	switch m.focused {
	case 0:
		m.yesterday.SetDelegate(delegate_focused)
		m.todos.SetDelegate(delegate_normal)
		m.weekly.SetDelegate(delegate_normal)
	case 1:
		m.yesterday.SetDelegate(delegate_normal)
		m.todos.SetDelegate(delegate_focused)
		m.weekly.SetDelegate(delegate_normal)
	default:
		m.yesterday.SetDelegate(delegate_normal)
		m.todos.SetDelegate(delegate_normal)
		m.weekly.SetDelegate(delegate_focused)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, docStyle.Render(m.yesterday.View()), docStyle.Render(m.todos.View()), docStyle.Render(m.weekly.View()))
}

func save(f_name string, items []list.Item) {
	tmp_file := f_name + ".tmp"
	file, err := os.Create(tmp_file)
	if err != nil {
		fmt.Println("Error creating temporary file:", err)
		os.Exit(1)
	}
	defer file.Close()

	t := time.Now()
	year, month, day := t.Date()
	s := fmt.Sprintf("%d,%d,%d\n", day, month, year)
	file.WriteString(s)

	for _, listItem := range items {
		i := listItem.(item)
		line := ""
		if i.marked {
			line = "✅ "
		}

		if i.command != "" {
			line = line + i.title + ", " + i.command
		} else {
			line = line + i.title
		}
		file.WriteString(line + "\n")
	}
	file.Close()
	err = os.Rename(tmp_file, f_name)
	if err != nil {
		fmt.Println("Error renaming temporary file:", err)
		os.Exit(1)
	}
}
func load(f_name string, configDir string) []list.Item {
	file, err := os.Open(f_name)
	if err != nil {
		if os.IsNotExist(err) {
			file, err = os.Create(f_name)
			if err != nil {
				fmt.Println("Error creating file:", err)
				os.Exit(1)
			}
		} else {
			fmt.Println("Error opening file:", err)
			os.Exit(1)
		}
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)

	l := []list.Item{}

	time_line := true
	different_day := false

	t := time.Now()

	year, month, day := t.Date()
	for scanner.Scan() {
		line := scanner.Text()

		if time_line {
			time_line = false
			time_list := strings.Split(line, ",")
			if len(time_list) == 3 {
				f_day, err_d := strconv.Atoi(time_list[0])
				f_month, err_m := strconv.Atoi(time_list[1])
				f_year, err_y := strconv.Atoi(time_list[2])
				if err_d == nil && err_m == nil && err_y == nil {
					if f_day != day || f_month != int(month) || f_year != year {
						different_day = true
					}
				}

				continue
			}
		}

		parts := strings.SplitN(line, ", ", 2)
		title := parts[0]
		command := ""
		if len(parts) > 1 {
			command = parts[1]
		}

		marked := false
		if strings.HasPrefix(title, "✅ ") {
			if different_day {
				log_file, err := os.OpenFile(filepath.Join(configDir, "unticked.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				if err != nil {
					fmt.Println("Error opening log file:", err)
				} else {
					log_t := t
					if f_name == "yesterday.txt" {
						log_t = t.AddDate(0, 0, -1)
					}
					log_year, log_month, log_day := log_t.Date()
					log_line := fmt.Sprintf("%d-%d-%d: %s\n", log_year, log_month, log_day, strings.TrimPrefix(title, "✅ "))
					log_file.WriteString(log_line)
					log_file.Close()
				}
			}
			marked = !different_day
			title = strings.TrimPrefix(title, "✅ ")
		}

		l = append(l, item{
			title:   title,
			marked:  marked,
			hidden:  false,
			command: command,
		})
	}
	return l
}

var delegate_normal = customDelegate{list.NewDefaultDelegate()}
var delegate_focused = customDelegate{list.NewDefaultDelegate()}

func getConfigDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	appConfigDir := filepath.Join(configDir, "accountability")
	if _, err := os.Stat(appConfigDir); os.IsNotExist(err) {
		if err := os.MkdirAll(appConfigDir, 0755); err != nil {
			return "", err
		}
	}
	return appConfigDir, nil
}
func filterCompleted(hide bool, allItems []list.Item) []list.Item {
	if !hide {
		return allItems
	}
	var filteredItems []list.Item
	for _, listItem := range allItems {
		if !listItem.(item).marked {
			filteredItems = append(filteredItems, listItem)
		}
	}
	return filteredItems
}

func main() {
	configDir, err := getConfigDir()
	if err != nil {
		fmt.Println("Error getting config directory:", err)
		os.Exit(1)
	}

	delegate_focused.Styles.SelectedTitle = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(lipgloss.Color("228")).
		Foreground(lipgloss.Color("228")).
		Padding(0, 0, 0, 2)
	delegate_focused.ShowDescription = false
	delegate_normal.ShowDescription = false

	yesterday_items := load(filepath.Join(configDir, "yesterday.txt"), configDir)
	todos_items := load(filepath.Join(configDir, "todos.txt"), configDir)
	weekly_items := load(filepath.Join(configDir, "weekly.txt"), configDir)
	m := model{
		yesterday:         list.New(yesterday_items, delegate_focused, 0, 0),
		todos:             list.New(todos_items, delegate_normal, 0, 0),
		weekly:            list.New(weekly_items, delegate_normal, 0, 0),
		focused:           0,
		configDir:         configDir,
		hideCompleted:     false,
		allItemsYesterday: yesterday_items,
		allItemsToday:     todos_items,
		ShowHelp:          false,
	}
	m.yesterday.Title = "Things (Hopefully) done yesterday"
	m.todos.Title = "Todays TODOS"
	m.weekly.Title = "Weekly Todos"
	m.yesterday.SetShowFilter(false)
	m.yesterday.SetFilteringEnabled(false)
	m.todos.SetShowFilter(false)
	m.todos.SetFilteringEnabled(false)
	m.weekly.SetShowFilter(false)
	m.weekly.SetFilteringEnabled(false)

	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		fmt.Println("Error running program:", err)
		os.Exit(1)
	}

	m, ok := finalModel.(model)
	if !ok {
		fmt.Println("Error getting final model")
		os.Exit(1)
	}

	save(filepath.Join(m.configDir, "yesterday.txt"), m.yesterday.Items())
	save(filepath.Join(m.configDir, "todos.txt"), m.todos.Items())
	save(filepath.Join(m.configDir, "weekly.txt"), m.weekly.Items())
}
