package config

import "strings"

var (
	Default = Config{
		WatchDirs: []ConfigDir{
			{
				Dir:        "$HOME/Downloads",
				FileSuffix: ".kubeconfig.txt",
				FileFormat: "cluster-bot-2006-01-02-150405.kubeconfig.txt",
				Lifespan:   150,
			},
		},
		KubeconfigLink: "$HOME/.cluster",
	}
)

type ConfigDir struct {
	Dir        string
	FileSuffix string
	FileFormat string
	Lifespan   uint // Number of minutes until this cluster expires (0 is never expire)
}

type Config struct {
	WatchDirs      []ConfigDir
	KubeconfigLink string
}

func (c *Config) Replace(o string, s string) {
	c.KubeconfigLink = strings.ReplaceAll(c.KubeconfigLink, o, s)
	for i := range c.WatchDirs {
		c.WatchDirs[i].Dir = strings.ReplaceAll(c.WatchDirs[i].Dir, o, s)
	}
}
