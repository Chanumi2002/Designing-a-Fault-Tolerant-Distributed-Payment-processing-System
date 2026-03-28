package fault

import (
	"log"
	"net"
	"strconv"
	"sync"
	"time"

	"distributed_payment_system/internal/config"
	"distributed_payment_system/internal/types"
)

// PeerSender is used to send messages to peer nodes
type PeerSender interface {
	Send(addr string, data []byte) error
}

// Detector handles heartbeat sending and peer failure checking
type Detector struct {
	Node   *types.Node
	Config *config.Config
	Sender PeerSender

	// Keeps current peer health states
	peerStatus map[string]types.NodeStatus
	mu         sync.RWMutex
}

// NewDetector creates a new failure detector
func NewDetector(node *types.Node, cfg *config.Config, sender PeerSender) *Detector {
	d := &Detector{
		Node:       node,
		Config:     cfg,
		Sender:     sender,
		peerStatus: make(map[string]types.NodeStatus),
	}

	// Start with all peers assumed alive
	for _, peer := range cfg.PeersExcluding(node.ID) {
		d.peerStatus[peer.ID] = types.StatusAlive
	}

	return d
}

// Start starts both background loops
func (d *Detector) Start() {
	d.StartHeartbeatLoop()
	d.StartFailureMonitor()
}

// StartHeartbeatLoop sends heartbeats at fixed intervals
func (d *Detector) StartHeartbeatLoop() {
	ticker := time.NewTicker(time.Duration(d.Config.HeartbeatInterval) * time.Second)

	go func() {
		defer ticker.Stop()

		for range ticker.C {
			d.sendHeartbeats()
		}
	}()
}

// sendHeartbeats sends a heartbeat message to every peer
func (d *Detector) sendHeartbeats() {
	peers := d.Config.PeersExcluding(d.Node.ID)

	for _, peer := range peers {
		payload := map[string]interface{}{
			"role":   string(d.Node.Role),
			"leader": d.Node.KnownLeader,
			"host":   d.Node.Host,
			"port":   d.Node.Port,
		}

		data, err := types.NewMessage(types.MsgHeartbeat, d.Node.ID, payload)
		if err != nil {
			log.Printf("heartbeat build failed for %s: %v\n", peer.ID, err)
			log.Printf("heartbeat sent from %s to %s\n", d.Node.ID, peer.ID)
			continue
		}

		addr := net.JoinHostPort(peer.Host, strconv.Itoa(peer.Port))
		if err := d.Sender.Send(addr, data); err != nil {
			log.Printf("heartbeat send failed to %s (%s): %v\n", peer.ID, addr, err)
		}
	}

}

// HandleHeartbeat updates last-seen time when a heartbeat arrives
func (d *Detector) HandleHeartbeat(msg *types.Message) {
	if msg == nil {
		return
	}

	d.Node.Mu.Lock()
	d.Node.LastHeartbeat[msg.Sender] = time.Now()

	// Update known leader if the sender included it
	if rawLeader, ok := msg.Payload["leader"]; ok {
		if leader, ok := rawLeader.(string); ok {
			d.Node.KnownLeader = leader
			log.Printf("heartbeat received at %s from %s\n", d.Node.ID, msg.Sender)
		}
	}
	d.Node.Mu.Unlock()

	d.mu.Lock()
	d.peerStatus[msg.Sender] = types.StatusAlive
	d.mu.Unlock()
}

// StartFailureMonitor checks periodically whether a peer has timed out
func (d *Detector) StartFailureMonitor() {
	ticker := time.NewTicker(1 * time.Second)

	go func() {
		defer ticker.Stop()

		for range ticker.C {
			d.checkFailures()
		}
	}()
}

// checkFailures marks peers as failed if heartbeat timeout is exceeded
func (d *Detector) checkFailures() {
	timeout := time.Duration(d.Config.HeartbeatTimeout) * time.Second
	now := time.Now()

	peers := d.Config.PeersExcluding(d.Node.ID)

	d.Node.Mu.RLock()
	defer d.Node.Mu.RUnlock()

	for _, peer := range peers {
		lastSeen, exists := d.Node.LastHeartbeat[peer.ID]
		if !exists {
			log.Printf("checking peer %s from node %s\n", peer.ID, d.Node.ID)
			continue
		}

		if now.Sub(lastSeen) > timeout {
			d.markPeerFailed(peer.ID)
		} else {
			d.markPeerAlive(peer.ID)
		}
	}
}

// markPeerFailed marks a peer as failed
func (d *Detector) markPeerFailed(peerID string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.peerStatus[peerID] == types.StatusFailed {
		return
	}

	d.peerStatus[peerID] = types.StatusFailed
	log.Printf("peer %s marked FAILED\n", peerID)
}

// markPeerAlive marks a peer as alive
func (d *Detector) markPeerAlive(peerID string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.peerStatus[peerID] == types.StatusAlive {
		return
	}

	d.peerStatus[peerID] = types.StatusAlive
	log.Printf("peer %s marked ALIVE\n", peerID)
}

// IsPeerAlive returns true if the peer is currently alive
func (d *Detector) IsPeerAlive(peerID string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.peerStatus[peerID] == types.StatusAlive
}

// GetPeerStatus gets one peer's status
func (d *Detector) GetPeerStatus(peerID string) types.NodeStatus {
	d.mu.RLock()
	defer d.mu.RUnlock()

	status, ok := d.peerStatus[peerID]
	if !ok {
		return types.StatusFailed
	}

	return status
}

// GetAllPeerStatuses returns a copy of all peer statuses
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
	statuses := d.GetAllPeerStatuses()
	log.Printf("node %s peer statuses: %+v\n", d.Node.ID, statuses)
}
