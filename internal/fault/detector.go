package fault

import (
	"net"
	"strconv"
	"sync"
	"time"

	"distributed_payment_system/internal/config"
	"distributed_payment_system/internal/types"
)

type PeerSender interface {
	Send(addr string, data []byte) error
}

type Detector struct {
	Node   *types.Node
	Config *config.Config
	Sender PeerSender

	peerStatus map[string]types.NodeStatus
	mu         sync.RWMutex
}

func NewDetector(node *types.Node, cfg *config.Config, sender PeerSender) *Detector {
	d := &Detector{
		Node:       node,
		Config:     cfg,
		Sender:     sender,
		peerStatus: make(map[string]types.NodeStatus),
	}

	for _, peer := range cfg.PeersExcluding(node.ID) {
		d.peerStatus[peer.ID] = types.StatusAlive
	}

	return d
}

func (d *Detector) Start() {
	d.StartHeartbeatLoop()
	d.StartFailureMonitor()
}

func (d *Detector) StartHeartbeatLoop() {
	ticker := time.NewTicker(time.Duration(d.Config.HeartbeatInterval) * time.Second)

	go func() {
		defer ticker.Stop()

		for range ticker.C {
			d.sendHeartbeats()
		}
	}()
}

func (d *Detector) sendHeartbeats() {
	d.Node.Mu.RLock()
	if !d.Node.IsActive || d.Node.Status == types.StatusFailed {
		d.Node.Mu.RUnlock()
		return
	}

	role := d.Node.Role
	leader := d.Node.KnownLeader
	host := d.Node.Host
	port := d.Node.Port
	d.Node.Mu.RUnlock()

	peers := d.Config.PeersExcluding(d.Node.ID)

	for _, peer := range peers {
		payload := map[string]interface{}{
			"role":   string(role),
			"leader": leader,
			"host":   host,
			"port":   port,
		}

		data, err := types.NewMessage(types.MsgHeartbeat, d.Node.ID, payload)
		if err != nil {
			continue
		}

		addr := net.JoinHostPort(peer.Host, strconv.Itoa(peer.Port))
		_ = d.Sender.Send(addr, data)
	}
}

func (d *Detector) HandleHeartbeat(msg *types.Message) {
	if msg == nil {
		return
	}

	d.Node.Mu.RLock()
	if !d.Node.IsActive || d.Node.Status == types.StatusFailed {
		d.Node.Mu.RUnlock()
		return
	}
	d.Node.Mu.RUnlock()

	d.Node.Mu.Lock()
	d.Node.LastHeartbeat[msg.Sender] = time.Now()

	if rawLeader, ok := msg.Payload["leader"]; ok {
		if leader, ok := rawLeader.(string); ok {
			d.Node.KnownLeader = leader
		}
	}
	d.Node.Mu.Unlock()

	d.mu.Lock()
	d.peerStatus[msg.Sender] = types.StatusAlive
	d.mu.Unlock()
}

func (d *Detector) StartFailureMonitor() {
	ticker := time.NewTicker(1 * time.Second)

	go func() {
		defer ticker.Stop()

		for range ticker.C {
			d.checkFailures()
		}
	}()
}

func (d *Detector) checkFailures() {
	d.Node.Mu.RLock()
	if !d.Node.IsActive || d.Node.Status == types.StatusFailed {
		d.Node.Mu.RUnlock()
		return
	}
	d.Node.Mu.RUnlock()

	timeout := time.Duration(d.Config.HeartbeatTimeout) * time.Second
	now := time.Now()

	peers := d.Config.PeersExcluding(d.Node.ID)

	d.Node.Mu.RLock()
	defer d.Node.Mu.RUnlock()

	for _, peer := range peers {
		lastSeen, exists := d.Node.LastHeartbeat[peer.ID]
		if !exists {
			continue
		}

		if now.Sub(lastSeen) > timeout {
			d.markPeerFailed(peer.ID)
		} else {
			d.markPeerAlive(peer.ID)
		}
	}
}

func (d *Detector) markPeerFailed(peerID string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.peerStatus[peerID] = types.StatusFailed
}

func (d *Detector) markPeerAlive(peerID string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.peerStatus[peerID] = types.StatusAlive
}

func (d *Detector) IsPeerAlive(peerID string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.peerStatus[peerID] == types.StatusAlive
}

func (d *Detector) GetPeerStatus(peerID string) types.NodeStatus {
	d.mu.RLock()
	defer d.mu.RUnlock()

	status, ok := d.peerStatus[peerID]
	if !ok {
		return types.StatusFailed
	}

	return status
}

func (d *Detector) GetAllPeerStatuses() map[string]types.NodeStatus {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make(map[string]types.NodeStatus)
	for id, status := range d.peerStatus {
		result[id] = status
	}
	return result
}

func (d *Detector) PrintStatuses() {
}
