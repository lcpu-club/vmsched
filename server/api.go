package server

import "time"

type GeneralResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type UserPut struct {
	Name    string         `json:"name"`
	Role    string         `json:"role"` // admin, user, banned
	Balance map[string]int `json:"balance"`
}

type TokenPut struct {
	Name   string `json:"name"`
	Secret string `json:"secret"`
}

type TokenGet []struct {
	Name string `json:"name"`
}

type TaskPost struct {
	Name         string `json:"name"`
	InstanceType string `json:"instance-type"`
}

type TaskStatePost struct {
	Status   string `json:"status"`
	LifeTime string `json:"life-time"`
}

type TaskGet struct {
	Name         string    `json:"name"`
	Instance     string    `json:"instance"`
	InstanceType string    `json:"instance-type"`
	Status       string    `json:"status"`
	Creation     time.Time `json:"creation"`
	QueueTime    time.Time `json:"queue-time"`
	EndTime      time.Time `json:"end-time"`
}

type InstanceTypeGet struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Price       map[string]int `json:"price"`
}

type InstanceTypePut struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Configure   string         `json:"configure"`
	Price       map[string]int `json:"price"`
}

type InstanceStatePut struct {
	Action   string `json:"action"`
	Force    bool   `json:"force"`
	Stateful bool   `json:"stateful"`
}

type InstanceStateGet struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	CPUUsage    int64  `json:"cpu-usage"`    // in nanoseconds
	MemoryUsage int64  `json:"memory-usage"` // in bytes
}

type QueueTimeGet struct {
	Duration string `json:"duration"`
}
