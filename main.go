package main

import (
	"encoding/json"
	"io/fs"
	"log"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dusk125/kcm/pkg/config"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

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
		return true
	}
	return time.Now().After(f.Time.Add(time.Minute * 150))
}

var (
	conf config.Config
)

type clusterList []fileInfo
type activeMsg string

type model struct {
	clusters clusterList
	cursor   int
	err      error
	active   string
}

func initialModel() *model {
	return &model{
		clusters: make(clusterList, 0),
	}
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
				if info.Time, err = time.Parse(wDir.FileFormat, info.Name); err != nil {
					return err
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
	return os.Symlink(m.clusters[m.cursor].Path(), conf.KubeconfigLink)
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(initialState, readActive)
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case clusterList:
		m.clusters = msg
		sort.Slice(m.clusters, func(i, j int) bool {
			return m.clusters[j].Time.After(m.clusters[i].Time)
		})
	case error:
		m.err = msg
	case activeMsg:
		m.active = string(msg)
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "j", "w":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "k", "s":
			if m.cursor < len(m.clusters)-1 {
				m.cursor++
			}
		case "enter", " ":
			return m, tea.Sequentially(rmActive, m.addActive, tea.Quit)
		case "r":
			return m, initialState
		case "d":
			return m, tea.Sequentially(m.clusters[m.cursor].Delete, initialState)
		case "c":
			if m.active != "" {
				return m, tea.Sequentially(rmActive, readActive)
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
			if cluster.Name == m.active {
				ss = append(ss, "[x]")
			} else {
				ss = append(ss, "[ ]")
			}
			if cluster.Expired() {
				ss = append(ss, "[EXPIRED]")
			}
			ss = append(ss, cluster.Name)
			s += strings.Join(ss, " ") + "\n"
		}
	}

	s += "\nPress r to refresh | Press d to delete | Press c to clear active | Press q to quit.\n"
	return s
}

func runTui() {
	if os.Getenv("KUBECONFIG") == "" {
		log.Fatalf("KUBECONFIG needs to be set:\n\texport KUBECONFIG=%v\n", conf.KubeconfigLink)
	}

	p := tea.NewProgram(initialModel())
	if err := p.Start(); err != nil {
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

	userConf := path.Join(home, ".kcm")

	if _, err = os.Stat(userConf); os.IsExist(err) {
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

		if err = json.NewEncoder(fi).Encode(&conf); err != nil {
			log.Fatalln(err)
		}
	}
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	cmd := &cobra.Command{
		Use:   "kcm",
		Short: "kcm a simple way to manage and use your kubeconfigs",
		Long:  `KubeConfg Manager allows you to keep track of your various kubeconfig files and easily switch between them`,
		Run: func(cmd *cobra.Command, args []string) {
			ensureConf()
			runTui()
		},
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List kubeconfig files found in config.WatchDirs",
		Run: func(cmd *cobra.Command, args []string) {
			ensureConf()
			switch msg := initialState().(type) {
			case error:
				log.Fatalln(msg)
			case clusterList:
				table := tablewriter.NewWriter(os.Stdout)
				table.SetHeader([]string{"Active", "Path", "Expired", "Created At", "Valid For"})
				for _, cluster := range msg {
					table.Append([]string{" ", cluster.Path(), strconv.FormatBool(cluster.Expired()), cluster.Time.String(), time.Duration(cluster.Lifespan * uint(time.Minute)).String()})
				}
				table.Render()
			}
		},
	})

	if err := cmd.Execute(); err != nil {
		log.Fatalln(err)
	}
}
