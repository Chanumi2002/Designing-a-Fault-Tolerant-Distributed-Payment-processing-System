package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"distributed_payment_system/internal/types"
)

type NodeView struct {
	ID          string `json:"id"`
	Role        string `json:"role"`
	Status      string `json:"status"`
	Port        int    `json:"port"`
	LastAction  string `json:"lastAction"`
	CurrentTerm int    `json:"currentTerm"`
}

type LogView struct {
	Time    string `json:"time"`
	Message string `json:"message"`
}

type DashboardResponse struct {
	Leader        string                `json:"leader"`
	Nodes         []NodeView            `json:"nodes"`
	Payments      []*types.PaymentEntry `json:"payments"`
	Logs          []LogView             `json:"logs"`
	NodesActive   string                `json:"nodesActive"`
	Committed     int                   `json:"committed"`
	Pending       int                   `json:"pending"`
	RecoveryState string                `json:"recoveryState"`
}

type CreatePaymentRequest struct {
	TransactionID string  `json:"transactionId"`
	Amount        float64 `json:"amount"`
	OwnerID       string  `json:"ownerId"`
}

type NodeActionRequest struct {
	NodeID string `json:"nodeId"`
}

type ActionResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type State struct {
	mu            sync.RWMutex
	leader        string
	currentTerm   int
	nodes         []NodeView
	payments      []*types.PaymentEntry
	logs          []LogView
	recoveryState string
}

type Handler struct {
	state *State
}

func NewHandler() *Handler {
	return &Handler{
		state: newState(),
	}
}

func newState() *State {
	s := &State{
		leader:      "node1",
		currentTerm: 0,
		nodes: []NodeView{
			{ID: "node1", Role: "Leader", Status: "Running", Port: 8001, LastAction: "Cluster initialized", CurrentTerm: 0},
			{ID: "node2", Role: "Follower", Status: "Running", Port: 8002, LastAction: "Waiting for leader", CurrentTerm: 0},
			{ID: "node3", Role: "Follower", Status: "Running", Port: 8003, LastAction: "Waiting for leader", CurrentTerm: 0},
		},
		payments:      []*types.PaymentEntry{},
		logs:          []LogView{},
		recoveryState: "Stable",
	}

	s.addLog("Dashboard API server started")
	s.addLog("Node1 is the current leader in term 0")
	return s
}

func (h *Handler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	h.state.mu.RLock()
	defer h.state.mu.RUnlock()

	resp := DashboardResponse{
		Leader:        h.state.leader,
		Nodes:         cloneNodes(h.state.nodes),
		Payments:      clonePayments(h.state.payments),
		Logs:          cloneLogs(h.state.logs),
		NodesActive:   h.state.nodesActiveText(),
		Committed:     h.state.committedCount(),
		Pending:       h.state.pendingCount(),
		RecoveryState: h.state.recoveryState,
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) GetLeader(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	h.state.mu.RLock()
	defer h.state.mu.RUnlock()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"leader":      h.state.leader,
		"currentTerm": h.state.currentTerm,
	})
}

func (h *Handler) GetNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	h.state.mu.RLock()
	defer h.state.mu.RUnlock()

	writeJSON(w, http.StatusOK, cloneNodes(h.state.nodes))
}

func (h *Handler) GetPayments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.state.mu.RLock()
		defer h.state.mu.RUnlock()
		writeJSON(w, http.StatusOK, clonePayments(h.state.payments))
	case http.MethodPost:
		h.CreatePayment(w, r)
	default:
		methodNotAllowed(w)
	}
}

func (h *Handler) GetLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	h.state.mu.RLock()
	defer h.state.mu.RUnlock()

	writeJSON(w, http.StatusOK, cloneLogs(h.state.logs))
}

func (h *Handler) CreatePayment(w http.ResponseWriter, r *http.Request) {
	var req CreatePaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ActionResponse{
			Success: false,
			Message: "invalid request body",
		})
		return
	}

	req.TransactionID = strings.TrimSpace(req.TransactionID)
	req.OwnerID = strings.TrimSpace(req.OwnerID)

	if req.TransactionID == "" || req.OwnerID == "" || req.Amount <= 0 {
		writeJSON(w, http.StatusBadRequest, ActionResponse{
			Success: false,
			Message: "transactionId, amount, and ownerId are required",
		})
		return
	}

	h.state.mu.Lock()
	defer h.state.mu.Unlock()

	if h.state.leader == "" {
		writeJSON(w, http.StatusConflict, ActionResponse{
			Success: false,
			Message: "no active leader available",
		})
		return
	}

	if !h.state.hasMajority() {
		h.state.addLog("Payment " + req.TransactionID + " rejected: majority not available")
		writeJSON(w, http.StatusConflict, ActionResponse{
			Success: false,
			Message: "cannot commit payment: majority not available",
		})
		return
	}

	for _, p := range h.state.payments {
		if p.TransactionID == req.TransactionID {
			writeJSON(w, http.StatusConflict, ActionResponse{
				Success: false,
				Message: "duplicate transaction ID",
			})
			return
		}
	}

	payment := &types.PaymentEntry{
		TransactionID: req.TransactionID,
		Amount:        req.Amount,
		Currency:      "USD",
		Status:        "committed",
		Timestamp:     float64(time.Now().UnixNano()) / 1e9,
		Version:       1,
		OwnerID:       req.OwnerID,
		StripeID:      "",
	}

	h.state.payments = append(h.state.payments, payment)
	h.state.setLeaderAction("Created "+req.TransactionID, "Replicated "+req.TransactionID)
	h.state.addLog(strings.ToUpper(h.state.leader) + " created payment " + req.TransactionID + " in term " + strconv.Itoa(h.state.currentTerm))
	h.state.addLog("Followers replicated " + req.TransactionID)
	h.state.addLog("Majority ACK received, " + req.TransactionID + " committed")

	writeJSON(w, http.StatusCreated, ActionResponse{
		Success: true,
		Message: "payment created successfully",
	})
}

func (h *Handler) FailNode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	var req NodeActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ActionResponse{
			Success: false,
			Message: "invalid request body",
		})
		return
	}

	nodeID := strings.TrimSpace(req.NodeID)
	if nodeID == "" {
		writeJSON(w, http.StatusBadRequest, ActionResponse{
			Success: false,
			Message: "nodeId is required",
		})
		return
	}

	h.state.mu.Lock()
	defer h.state.mu.Unlock()

	node := h.state.findNode(nodeID)
	if node == nil {
		writeJSON(w, http.StatusNotFound, ActionResponse{
			Success: false,
			Message: "node not found",
		})
		return
	}

	if node.Status == "Failed" {
		writeJSON(w, http.StatusConflict, ActionResponse{
			Success: false,
			Message: nodeID + " is already failed",
		})
		return
	}

	wasLeader := h.state.leader == nodeID

	h.state.markNodeFailed(nodeID)
	h.state.addLog(strings.ToUpper(nodeID) + " failed")

	if wasLeader {
		h.state.addLog(strings.ToUpper(nodeID) + " was leader, triggering failover")
		nextLeader, err := h.state.promoteNextLeader(nodeID)
		if err != nil {
			writeJSON(w, http.StatusOK, ActionResponse{
				Success: true,
				Message: nodeID + " failed, no new leader available",
			})
			return
		}

		h.state.addLog(strings.ToUpper(nextLeader) + " became the new leader in term " + strconv.Itoa(h.state.currentTerm))
		writeJSON(w, http.StatusOK, ActionResponse{
			Success: true,
			Message: nodeID + " failed, " + nextLeader + " is now leader",
		})
		return
	}

	writeJSON(w, http.StatusOK, ActionResponse{
		Success: true,
		Message: nodeID + " failed successfully",
	})
}

func (h *Handler) RejoinNode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	var req NodeActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ActionResponse{
			Success: false,
			Message: "invalid request body",
		})
		return
	}

	nodeID := strings.TrimSpace(req.NodeID)
	if nodeID == "" {
		writeJSON(w, http.StatusBadRequest, ActionResponse{
			Success: false,
			Message: "nodeId is required",
		})
		return
	}

	h.state.mu.Lock()
	defer h.state.mu.Unlock()

	node := h.state.findNode(nodeID)
	if node == nil {
		writeJSON(w, http.StatusNotFound, ActionResponse{
			Success: false,
			Message: "node not found",
		})
		return
	}

	if node.Status != "Failed" {
		writeJSON(w, http.StatusConflict, ActionResponse{
			Success: false,
			Message: nodeID + " is already running",
		})
		return
	}

	node.Status = "Running"
	node.CurrentTerm = h.state.currentTerm
	if h.state.leader == nodeID {
		node.Role = "Leader"
	} else {
		node.Role = "Follower"
	}
	node.LastAction = "Recovered missing transactions"

	h.state.recoveryState = "Recovered"
	h.state.addLog(strings.ToUpper(nodeID) + " rejoined the cluster in term " + strconv.Itoa(h.state.currentTerm))
	h.state.addLog("Recovery request sent for " + nodeID)

	if len(h.state.payments) == 0 {
		h.state.addLog("Recovery completed for " + nodeID + ", no payments were missing")
	} else {
		for _, p := range h.state.payments {
			h.state.addLog("Recovery applied for " + p.TransactionID + " on " + nodeID)
		}
	}

	writeJSON(w, http.StatusOK, ActionResponse{
		Success: true,
		Message: nodeID + " rejoined successfully",
	})
}

func (s *State) addLog(message string) {
	log.Println(message)

	s.logs = append(s.logs, LogView{
		Time:    time.Now().Format("15:04:05"),
		Message: message,
	})

	if len(s.logs) > 200 {
		s.logs = s.logs[len(s.logs)-200:]
	}
}

func (s *State) nodesActiveText() string {
	active := s.runningNodesCount()
	return itoa(active) + " / " + itoa(len(s.nodes))
}

func (s *State) runningNodesCount() int {
	active := 0
	for _, n := range s.nodes {
		if n.Status == "Running" {
			active++
		}
	}
	return active
}

func (s *State) hasMajority() bool {
	required := (len(s.nodes) / 2) + 1
	return s.runningNodesCount() >= required
}

func (s *State) committedCount() int {
	count := 0
	for _, p := range s.payments {
		if p.Status == "committed" {
			count++
		}
	}
	return count
}

func (s *State) pendingCount() int {
	count := 0
	for _, p := range s.payments {
		if p.Status == "pending" {
			count++
		}
	}
	return count
}

func (s *State) setLeaderAction(leaderAction, followerAction string) {
	for i := range s.nodes {
		switch s.nodes[i].ID {
		case s.leader:
			s.nodes[i].LastAction = leaderAction
			s.nodes[i].Role = "Leader"
			s.nodes[i].Status = "Running"
			s.nodes[i].CurrentTerm = s.currentTerm
		default:
			if s.nodes[i].Status == "Running" {
				s.nodes[i].Role = "Follower"
				s.nodes[i].LastAction = followerAction
				s.nodes[i].CurrentTerm = s.currentTerm
			}
		}
	}
}

func (s *State) markNodeFailed(nodeID string) {
	for i := range s.nodes {
		if s.nodes[i].ID == nodeID {
			s.nodes[i].Status = "Failed"
			s.nodes[i].Role = "Failed"
			s.nodes[i].LastAction = "Node unavailable"
			s.nodes[i].CurrentTerm = s.currentTerm
			break
		}
	}

	if s.leader == nodeID {
		s.leader = ""
	}
}

func (s *State) promoteNextLeader(excluding string) (string, error) {
	order := []string{"node1", "node2", "node3"}

	s.currentTerm++

	for _, id := range order {
		if id == excluding {
			continue
		}

		for i := range s.nodes {
			if s.nodes[i].ID == id && s.nodes[i].Status == "Running" {
				s.leader = id
				s.nodes[i].Role = "Leader"
				s.nodes[i].LastAction = "Took over leadership"
				s.nodes[i].CurrentTerm = s.currentTerm

				for j := range s.nodes {
					if s.nodes[j].ID != id {
						s.nodes[j].CurrentTerm = s.currentTerm
						if s.nodes[j].Status == "Running" {
							s.nodes[j].Role = "Follower"
						}
					}
				}

				return id, nil
			}
		}
	}

	s.leader = ""
	return "", errors.New("no running follower available to become leader")
}

func (s *State) findNode(nodeID string) *NodeView {
	for i := range s.nodes {
		if s.nodes[i].ID == nodeID {
			return &s.nodes[i]
		}
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func methodNotAllowed(w http.ResponseWriter) {
	writeJSON(w, http.StatusMethodNotAllowed, ActionResponse{
		Success: false,
		Message: "method not allowed",
	})
}

func cloneNodes(nodes []NodeView) []NodeView {
	out := make([]NodeView, len(nodes))
	copy(out, nodes)
	return out
}

func cloneLogs(logs []LogView) []LogView {
	out := make([]LogView, len(logs))
	copy(out, logs)
	return out
}

func clonePayments(payments []*types.PaymentEntry) []*types.PaymentEntry {
	out := make([]*types.PaymentEntry, 0, len(payments))
	for _, p := range payments {
		cp := *p
		out = append(out, &cp)
	}
	return out
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
