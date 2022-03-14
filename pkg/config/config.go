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
		KubeconfigLink: "$HOME/.kcm-active",
	}
)

type ConfigDir struct {
	// The directory to search in for kubeconfig files. (will substitute $HOME to the users home directory)
	Dir string
	// The full extension (.kubeconfig.txt) of the files you're looking for.
	FileSuffix string
	// A time.Parse version of the filename you're expecting; is used with Lifespan to show whether a kubeconfig is expired or not.
	FileFormat string
	// Number of minutes until this cluster expires (0 is never expire).
	Lifespan uint
}

type Config struct {
	// List of directories to watch for kubeconfig files.
	WatchDirs []ConfigDir
	// The destintion of kubeconfig symlink; your KUBECONFIG env var should be set this to value as well.
	// (Will substitute $HOME to the users home directory)
	KubeconfigLink string
}

func (c *Config) Replace(o string, s string) {
	c.KubeconfigLink = strings.ReplaceAll(c.KubeconfigLink, o, s)
	for i := range c.WatchDirs {
		c.WatchDirs[i].Dir = strings.ReplaceAll(c.WatchDirs[i].Dir, o, s)
	}
}
