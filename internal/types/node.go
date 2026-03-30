package types

import (
	"sync"
	"time"
)

type NodeStatus string
type NodeRole string

const (
	StatusAlive  NodeStatus = "alive"
	StatusFailed NodeStatus = "failed"
	RoleLeader   NodeRole   = "leader"
	RoleFollower NodeRole   = "follower"
)

type Node struct {
	ID            string
	Host          string
	Port          int
	Status        NodeStatus
	Role          NodeRole
	ClockOffset   float64
	Payments      map[string]*PaymentEntry
	LastHeartbeat map[string]time.Time
	KnownLeader   string
	IsActive      bool
	Mu            sync.RWMutex
}

func NewNode(id, host string, port int) *Node {
	return &Node{
		ID:            id,
		Host:          host,
		Port:          port,
		Status:        StatusAlive,
		Role:          RoleFollower,
		Payments:      make(map[string]*PaymentEntry),
		LastHeartbeat: make(map[string]time.Time),
		IsActive:      true,
	}
}
