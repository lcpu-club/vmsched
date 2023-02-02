package models

import (
	"time"

	"github.com/lcpu-dev/vmsched/renderer"
)

type Task struct {
	Name         string    `xorm:"name pk notnull"`
	InstanceType string    `xorm:"instance_type notnull"`
	Creation     time.Time `xorm:"creation created"`
	QueueTime    time.Time `xorm:"queue_time"`
	EndTime      time.Time `xorm:"end_time"`
	Status       string    `xorm:"status"` // active, queued, terminating, inactive, creating, deleting
	TargetID     int64     `xorm:"target_id"`
	Instance     string    `xorm:"instance"`
	User         string    `xorm:"user notnull"`
	Version      int       `xorm:"version"`
}

type User struct {
	Name    string         `xorm:"name pk notnull"`
	Role    string         `xorm:"role"` // admin, user, banned
	Balance map[string]int `xorm:"balance json"`
	Version int            `xorm:"version"`
}

type Token struct {
	Name   string `xorm:"name pk"`
	Secret string `xorm:"secret varchar(512)"`
	User   string `xorm:"user"`
}

type InstanceType struct {
	Name        string         `xorm:"name pk notnull"`
	Description string         `xorm:"description text"`
	Configure   string         `xorm:"configure text"`
	Price       map[string]int `xorm:"price json"` // price per minute
}

type InstanceTarget struct {
	Id       int64            `xorm:"'id' pk autoincr"`
	Type     string           `xorm:"type"`
	Target   *renderer.Target `xorm:"target json"`
	Status   string           `xorm:"status"` // busy or idle
	Instance string           `xorm:"instance"`
	Task     string           `xorm:"task"`
	Version  int              `xorm:"version"`
}

type Queue struct {
	Id           int64         `xorm:"'id' pk autoincr"`
	User         string        `xorm:"user notnull"`
	Task         string        `xorm:"task notnull"`
	InstanceType string        `xorm:"instance_type notnull"`
	LifeTime     time.Duration `xorm:"life_time"`
	Creation     time.Time     `xorm:"creation created"`
}
