package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Configure struct {
	Listen       string             `yaml:"listen" json:"listen"`
	LXD          *LXDConfigure      `yaml:"lxd" json:"lxd"`
	Database     *DatabaseConfigure `yaml:"database" json:"database"`
	CronInterval time.Duration      `yaml:"cron-interval" json:"cron-interval"`
}

type LXDConfigure struct {
	Address    string `yaml:"address" json:"address"`
	ClientKey  string `yaml:"client-key" json:"client-key"`
	ClientCert string `yaml:"client-cert" json:"client-cert"`
}

type DatabaseConfigure struct {
	Driver string `yaml:"driver" json:"driver"`
	DSN    string `yaml:"dsn" json:"dsn"`
}

func LoadConfigure(path string) (*Configure, error) {
	f, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	r := new(Configure)
	err = yaml.Unmarshal(f, r)
	if err != nil {
		return nil, err
	}
	if r.LXD == nil {
		r.LXD = new(LXDConfigure)
	}
	if r.Database == nil {
		r.Database = new(DatabaseConfigure)
	}
	return r, nil
}
