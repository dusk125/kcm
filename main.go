package main

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type fileInfo struct {
	Name string
	Time time.Time
}

type clusterList []fileInfo
type activeMsg string

type model struct {
	clusters clusterList
	cursor   int
	err      error
	active   string
}

func initialModel() model {
	return model{
		clusters: make(clusterList, 0),
	}
}

func initialState() tea.Msg {
	var (
		err   error
		files []fs.DirEntry
	)
	if files, err = os.ReadDir("/home/alray/Downloads"); err != nil {
		return err
	}

	clusters := clusterList{}
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".kubeconfig.txt") {
			info := fileInfo{
				Name: file.Name(),
			}
			if i, err := file.Info(); err != nil {
				return err
			} else if info.Time, err = time.Parse("2006-01-02-150405", strings.TrimSuffix(strings.TrimPrefix(i.Name(), "cluster-bot-"), ".kubeconfig.txt")); err != nil {
				return err
			}
			clusters = append(clusters, info)
		}
	}

	return clusters
}

func readActive() tea.Msg {
	var (
		err    error
		active string
	)
	if active, err = os.Readlink("/home/alray/.cluster"); err != nil {
		return ""
	}
	return activeMsg(path.Base(active))
}

func (m model) Init() tea.Cmd {
	return tea.Batch(initialState, readActive)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			if _, m.err = os.Stat("/home/alray/.cluster"); os.IsExist(m.err) {
				if m.err = os.Remove("/home/alray/.cluster"); m.err != nil {
					return m, nil
				}
			}
			m.err = os.Symlink(path.Join("/home/alray/Downloads", m.clusters[m.cursor].Name), "/home/alray/.cluster")
			return m, readActive
		case "r":
			return m, initialState
		case "d":
			m.err = os.Remove(path.Join("/home/alray/Downloads", m.clusters[m.cursor].Name))
			return m, initialState
		case "c":
			if m.active != "" {
				_ = os.Remove("/home/alray/.cluster")
				return m, readActive
			}
		}
	}
	return m, nil
}

func (m model) View() string {
	var s string
	if m.err != nil {
		s = m.err.Error()
	} else {
		s = "Select a cluster.\n\n"

		if m.active != "" {
			s += fmt.Sprintf("Currently Active:\n\t%s\n\n", m.active)
		}

		for i, cluster := range m.clusters {
			cursor := " "
			if m.cursor == i {
				cursor = ">"
			}
			f := "%s %s"
			if time.Now().After(cluster.Time.Add(time.Minute * 150)) {
				f = "%s [EXPIRED] %s"
			}
			s += fmt.Sprintf(f+"\n", cursor, cluster.Name)
		}
	}

	s += "\nPress r to refresh | Press d to delete | Press q to quit.\n"
	return s
}

func main() {
	p := tea.NewProgram(initialModel())
	if err := p.Start(); err != nil {
		log.Fatalln(err)
	}
}
