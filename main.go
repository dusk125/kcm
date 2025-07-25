package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dusk125/kcm/pkg/config"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

type keyMap struct {
	Up      key.Binding
	Down    key.Binding
	Refresh key.Binding
	Quit    key.Binding
	Delete  key.Binding
	Clear   key.Binding
	Select  key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Select, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Refresh, k.Clear, k.Delete},
		k.ShortHelp(),
	}
}

var (
	keys = keyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "w"),
			key.WithHelp("↑/w", "move up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "s"),
			key.WithHelp("↓/s", "move down"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q/ctrl+c", "quit"),
		),
		Delete: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "delete"),
		),
		Clear: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "clear"),
		),
		Select: key.NewBinding(
			key.WithKeys("e", "enter", " "),
			key.WithHelp("e/enter/space", "select"),
		),
	}
)

func fmtYesNo(v bool) string {
	if v {
		return "yes"
	} else {
		return "no"
	}
}

type fileInfo struct {
	Name     string
	Dir      string
	Time     time.Time
	Lifespan uint
}

func (f fileInfo) Path() string {
	return path.Join(f.Dir, f.Name)
}

func (f fileInfo) Delete() tea.Msg {
	return os.Remove(f.Path())
}

func (f fileInfo) Expired() bool {
	if f.Lifespan == 0 {
		return false
	}
	return time.Now().After(f.Time.Add(time.Minute * 150))
}

var (
	conf      config.Config
	greenText = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	redText   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
)

type clusterList []fileInfo
type activeMsg string

type model struct {
	keys     keyMap
	help     help.Model
	clusters clusterList
	cursor   int
	err      error
	active   string
}

func initialModel() (m *model) {
	m = &model{
		keys:     keys,
		help:     help.New(),
		clusters: make(clusterList, 0),
	}
	return
}

func initialState() tea.Msg {
	var (
		err      error
		files    []fs.DirEntry
		clusters = clusterList{}
	)

	for _, wDir := range conf.WatchDirs {
		if files, err = os.ReadDir(wDir.Dir); err != nil {
			return err
		}

		for _, file := range files {
			if !file.IsDir() && strings.HasSuffix(file.Name(), wDir.FileSuffix) {
				info := fileInfo{
					Dir:      wDir.Dir,
					Name:     file.Name(),
					Lifespan: wDir.Lifespan,
				}
				if wDir.FileFormat != "" {
					if info.Time, err = time.Parse(wDir.FileFormat, info.Name); err != nil {
						return err
					}
				}
				clusters = append(clusters, info)
			}
		}
	}

	return clusters
}

func readActive() tea.Msg {
	var (
		err    error
		active string
	)
	if active, err = os.Readlink(conf.KubeconfigLink); err != nil {
		return activeMsg("")
	}
	return activeMsg(path.Base(active))
}

func rmActive() tea.Msg {
	_ = os.Remove(conf.KubeconfigLink)
	return nil
}

func (m *model) addActive() tea.Msg {
	if m.cursor >= 0 && m.cursor < len(m.clusters) {
		cluster := m.clusters[m.cursor]
		fmt.Printf("%v is now your active kubeconfig.\n", cluster.Name)
		return os.Symlink(cluster.Path(), conf.KubeconfigLink)
	}
	return nil
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(initialState, readActive)
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case clusterList:
		m.clusters = msg
		slices.SortFunc(m.clusters, func(a, b fileInfo) int {
			aExpired, bExpired := a.Expired(), b.Expired()
			if aExpired != bExpired {
				switch {
				case aExpired:
					return -1
				case bExpired:
					return 1
				}
			}
			return b.Time.Compare(a.Time)
		})
	case error:
		m.err = msg
	case activeMsg:
		m.active = string(msg)
	case tea.WindowSizeMsg:
		m.help.Width = msg.Width
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(msg, m.keys.Down):
			if m.cursor < len(m.clusters)-1 {
				m.cursor++
			}
		case key.Matches(msg, m.keys.Select):
			return m, tea.Sequence(rmActive, m.addActive, tea.Quit)
		case key.Matches(msg, m.keys.Refresh):
			return m, initialState
		case key.Matches(msg, m.keys.Delete):
			if len(m.clusters) != 0 {
				cmds := []tea.Cmd{m.clusters[m.cursor].Delete, initialState}
				if active := readActive().(activeMsg); string(active) == m.clusters[m.cursor].Name {
					cmds = append([]tea.Cmd{rmActive}, cmds...)
				}
				return m, tea.Sequence(cmds...)
			}
		case key.Matches(msg, m.keys.Clear):
			if m.active != "" {
				return m, tea.Sequence(rmActive, readActive)
			}
		}
	}
	return m, nil
}

func (m *model) View() string {
	var s string
	if m.err != nil {
		s = m.err.Error()
	} else {
		s = "Select a cluster.\n\n"

		for i, cluster := range m.clusters {
			cursor := " "
			if m.cursor == i {
				cursor = ">"
			}
			ss := []string{cursor}
			active := " "
			if cluster.Name == m.active {
				active = greenText.Render("x")
			}
			ss = append(ss, fmt.Sprintf("[%v]", active))
			if cluster.Expired() {
				ss = append(ss, fmt.Sprintf("[%v]", redText.Render("EXPIRED")))
			}
			ss = append(ss, cluster.Name)
			s += strings.Join(ss, " ") + "\n"
		}
	}

	helpview := m.help.FullHelpView(m.keys.FullHelp())

	return s + "\n\n" + helpview + "\n"
}

func runTui() {
	if os.Getenv("KUBECONFIG") == "" {
		fmt.Printf("KUBECONFIG needs to be set:\n\texport KUBECONFIG=%v\n", conf.KubeconfigLink)
		os.Exit(0)
	}

	p := tea.NewProgram(initialModel())
	if _, err := p.Run(); err != nil {
		log.Fatalln(err)
	}
}

func ensureConf() {
	var (
		err  error
		home string
	)

	if home, err = os.UserHomeDir(); err != nil {
		log.Fatalln(err)
	}

	var (
		info     fs.FileInfo
		userConf = path.Join(home, ".kcm")
	)
	if info, err = os.Stat(userConf); os.IsExist(err) || (info != nil && info.Name() != "") {
		var (
			fi *os.File
		)

		if fi, err = os.Open(userConf); err != nil {
			log.Fatalln(err)
		}
		defer fi.Close()

		if err = json.NewDecoder(fi).Decode(&conf); err != nil {
			log.Fatalln(err)
		}
	} else {
		var (
			fi *os.File
		)

		if fi, err = os.Create(userConf); err != nil {
			log.Fatalln(err)
		}
		defer fi.Close()

		conf = config.Default
		conf.Replace("$HOME", home)

		enc := json.NewEncoder(fi)
		enc.SetIndent("", "\t")
		if err = enc.Encode(&conf); err != nil {
			log.Fatalln(err)
		}
	}
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Run kcm selector interface; can also just run `kcm` with no subcommands",
		Run: func(cmd *cobra.Command, args []string) {
			ensureConf()
			runTui()
		},
	}

	cmd := &cobra.Command{
		Use:   "kcm",
		Short: "kcm a simple way to manage and use your kubeconfigs",
		Long:  `KubeConfg Manager allows you to keep track of your various kubeconfig files and easily switch between them`,
		Run:   runCmd.Run,
	}

	cmd.AddCommand(runCmd)

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List kubeconfig files found in config.WatchDirs",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			ensureConf()
			switch msg := initialState().(type) {
			case error:
				return msg
			case clusterList:
				var active string
				switch msg := readActive().(type) {
				case activeMsg:
					active = string(msg)
				}

				table := tablewriter.NewWriter(os.Stdout)
				table.Header("Active", "Path", "Expired", "Created At", "Valid For")
				for _, cluster := range msg {
					var a string
					if active == cluster.Name {
						a = greenText.Render("yes")
					} else {
						a = ""
					}
					expired := redText.Render(fmtYesNo(cluster.Expired()))
					if err := table.Append(a, cluster.Path(), expired, cluster.Time.String(), time.Duration(cluster.Lifespan*uint(time.Minute)).String()); err != nil {
						return fmt.Errorf("failed to append to the table: %v", err)
					}
				}
				if err := table.Render(); err != nil {
					return fmt.Errorf("failed to render the table: %v", err)
				}
			}
			return nil
		},
	})

	if err := cmd.Execute(); err != nil {
		log.Fatalln(err)
	}
}
