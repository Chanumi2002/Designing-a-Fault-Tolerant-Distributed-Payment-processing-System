package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"distributed_payment_system/internal/transport"
	"distributed_payment_system/internal/types"
	"distributed_payment_system/internal/utils"
)

const apiServerLogFile = "api_server.log"

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

type committedPaymentInfo struct {
	TransactionID string
	Amount        float64
	OwnerID       string
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
	existingLogs := loadExistingDashboardLogs()

	s := &State{
		leader:      "",
		currentTerm: 0,
		nodes: []NodeView{
			{ID: "node1", Role: "Offline", Status: "Offline", Port: 8001, LastAction: "Node not started", CurrentTerm: 0},
			{ID: "node2", Role: "Offline", Status: "Offline", Port: 8002, LastAction: "Node not started", CurrentTerm: 0},
			{ID: "node3", Role: "Offline", Status: "Offline", Port: 8003, LastAction: "Node not started", CurrentTerm: 0},
		},
		payments:      []*types.PaymentEntry{},
		logs:          existingLogs,
		recoveryState: "Stable",
	}

	s.refreshNodeStartupState()
	if s.leader != "" {
		s.addLog(strings.ToUpper(s.leader) + " detected as current leader at startup")
	} else {
		s.addLog("Dashboard API server started")
		s.addLog("No active nodes detected yet")
	}

	return s
}

func loadExistingDashboardLogs() []LogView {
	storedLogs, err := utils.ReadPersistedLogs(apiServerLogFile, 200)
	if err != nil {
		return []LogView{}
	}

	logs := make([]LogView, 0, len(storedLogs))
	for _, entry := range storedLogs {
		logs = append(logs, LogView{
			Time:    entry.Time,
			Message: entry.Message,
		})
	}

	return logs
}

func sendControlMessage(node NodeView, msgType string) {
	client := transport.NewUDPClient()

	payload := map[string]interface{}{
		"node_id": node.ID,
	}

	data, err := types.NewMessage(msgType, "api_server", payload)
	if err != nil {
		return
	}

	addr := "127.0.0.1:" + strconv.Itoa(node.Port)

	for i := 0; i < 5; i++ {
		_ = client.Send(addr, data)
		time.Sleep(60 * time.Millisecond)
	}
}

func (h *Handler) notifyLeaderChange() {
	client := transport.NewUDPClient()

	for _, node := range h.state.nodes {
		if node.Status != "Running" {
			continue
		}

		payload := map[string]interface{}{
			"leader_id": h.state.leader,
			"term":      h.state.currentTerm,
		}

		data, err := types.NewMessage(types.MsgLeaderChange, "api_server", payload)
		if err != nil {
			continue
		}

		addr := "127.0.0.1:" + strconv.Itoa(node.Port)
		_ = client.Send(addr, data)
	}
}

func (h *Handler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	h.state.mu.Lock()
	h.state.refreshNodeStartupState()
	h.state.syncPaymentsFromNodeLogs()

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
	h.state.mu.Unlock()

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) GetLeader(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	h.state.mu.Lock()
	h.state.refreshNodeStartupState()
	defer h.state.mu.Unlock()

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

	h.state.mu.Lock()
	h.state.refreshNodeStartupState()
	nodes := cloneNodes(h.state.nodes)
	h.state.mu.Unlock()

	writeJSON(w, http.StatusOK, nodes)
}

func (h *Handler) GetPayments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.state.mu.Lock()
		h.state.refreshNodeStartupState()
		h.state.syncPaymentsFromNodeLogs()
		payments := clonePayments(h.state.payments)
		h.state.mu.Unlock()
		writeJSON(w, http.StatusOK, payments)
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

	h.state.refreshNodeStartupState()
	h.state.syncPaymentsFromNodeLogs()

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
			Message: "cannot process payment: majority not available",
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

	leaderNode := h.state.findNode(h.state.leader)
	if leaderNode == nil || leaderNode.Status != "Running" {
		writeJSON(w, http.StatusConflict, ActionResponse{
			Success: false,
			Message: "leader node not available",
		})
		return
	}

	payload := map[string]interface{}{
		"transaction_id": req.TransactionID,
		"amount":         req.Amount,
		"owner_id":       req.OwnerID,
	}

	data, err := types.NewMessage(types.MsgPaymentCreate, "api_server", payload)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ActionResponse{
			Success: false,
			Message: "failed to build payment message",
		})
		return
	}

	client := transport.NewUDPClient()
	leaderAddr := "127.0.0.1:" + strconv.Itoa(leaderNode.Port)
	if err := client.Send(leaderAddr, data); err != nil {
		writeJSON(w, http.StatusBadGateway, ActionResponse{
			Success: false,
			Message: "failed to send payment to leader",
		})
		return
	}

	payment := &types.PaymentEntry{
		TransactionID: req.TransactionID,
		Amount:        req.Amount,
		Currency:      "USD",
		Status:        "pending",
		Timestamp:     float64(time.Now().UnixNano()) / 1e9,
		Version:       1,
		OwnerID:       req.OwnerID,
		StripeID:      "",
	}

	h.state.payments = append(h.state.payments, payment)
	h.state.setLeaderAction("Processing "+req.TransactionID, "Waiting for commit")
	h.state.addLog("Payment " + req.TransactionID + " sent to leader")

	writeJSON(w, http.StatusAccepted, ActionResponse{
		Success: true,
		Message: "payment request sent to leader",
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

	h.state.refreshNodeStartupState()

	node := h.state.findNode(nodeID)
	if node == nil {
		writeJSON(w, http.StatusNotFound, ActionResponse{
			Success: false,
			Message: "node not found",
		})
		return
	}

	if node.Status == "Offline" {
		writeJSON(w, http.StatusConflict, ActionResponse{
			Success: false,
			Message: nodeID + " is not started",
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

	sendControlMessage(*node, types.MsgNodeFail)

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

		h.notifyLeaderChange()
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

	if node.Status == "Offline" {
		writeJSON(w, http.StatusConflict, ActionResponse{
			Success: false,
			Message: nodeID + " is not started yet. Run the node command first.",
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

	sendControlMessage(*node, types.MsgNodeRejoin)

	node.Status = "Running"
	node.CurrentTerm = h.state.currentTerm
	if h.state.leader == nodeID {
		node.Role = "Leader"
	} else {
		node.Role = "Follower"
	}
	node.LastAction = "Recovery requested"

	h.notifyLeaderChange()

	h.state.recoveryState = "Recovered"
	h.state.addLog(strings.ToUpper(nodeID) + " rejoined the cluster in term " + strconv.Itoa(h.state.currentTerm))
	h.state.addLog("Recovery request sent for " + nodeID)

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

func (s *State) syncPaymentsFromNodeLogs() {
	committed := loadCommittedPaymentsFromNodeLogs(s.runningNodeLogFiles())
	if len(committed) == 0 {
		return
	}

	existing := make(map[string]*types.PaymentEntry)
	for _, payment := range s.payments {
		existing[payment.TransactionID] = payment
	}

	for txnID, info := range committed {
		if payment, ok := existing[txnID]; ok {
			payment.Status = "committed"
			if info.Amount > 0 {
				payment.Amount = info.Amount
			}
			if info.OwnerID != "" {
				payment.OwnerID = info.OwnerID
			}
			continue
		}

		s.payments = append(s.payments, &types.PaymentEntry{
			TransactionID: info.TransactionID,
			Amount:        info.Amount,
			Currency:      "USD",
			Status:        "committed",
			Timestamp:     float64(time.Now().UnixNano()) / 1e9,
			Version:       1,
			OwnerID:       info.OwnerID,
			StripeID:      "",
		})
	}
}

func (s *State) runningNodeLogFiles() []string {
	files := make([]string, 0, len(s.nodes))

	for _, node := range s.nodes {
		if node.Status != "Running" {
			continue
		}

		files = append(files, node.ID+".log")
	}

	return files
}

func loadCommittedPaymentsFromNodeLogs(files []string) map[string]committedPaymentInfo {
	committed := make(map[string]committedPaymentInfo)

	for _, file := range files {
		lines, err := utils.ReadPersistedLogs(file, 500)
		if err != nil {
			continue
		}

		for _, line := range lines {
			info, ok := parseCommittedPaymentLog(line.Message)
			if !ok {
				continue
			}
			committed[info.TransactionID] = info
		}
	}

	return committed
}

func parseCommittedPaymentLog(message string) (committedPaymentInfo, bool) {
	parts := strings.Split(message, "|")
	info := committedPaymentInfo{}

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}

		key := strings.TrimSpace(strings.ToLower(kv[0]))
		value := strings.TrimSpace(kv[1])

		switch key {
		case "txn":
			info.TransactionID = value
		case "amount":
			amount, err := strconv.ParseFloat(value, 64)
			if err == nil {
				info.Amount = amount
			}
		case "owner":
			info.OwnerID = value
		}
	}

	if info.TransactionID == "" {
		return committedPaymentInfo{}, false
	}

	return info, true
}

func (s *State) refreshNodeStartupState() {
	for i := range s.nodes {
		node := &s.nodes[i]

		if node.Status == "Failed" {
			continue
		}

		if hasNodeStarted(node.ID) {
			if node.Status == "Offline" {
				node.Status = "Running"
				node.LastAction = "Node started"
				node.CurrentTerm = 0
			}

			if node.Role == "Offline" {
				node.Role = "Follower"
			}
		} else {
			node.Status = "Offline"
			node.Role = "Offline"
			node.LastAction = "Node not started"
			node.CurrentTerm = 0
		}
	}

	s.recalculateLeaderFromRunningNodes()
}

func hasNodeStarted(nodeID string) bool {
	filename := nodeID + ".log"
	path := utils.GetLogFilePath(filename)

	if _, err := os.Stat(path); err != nil {
		return false
	}

	lines, err := utils.ReadPersistedLogs(filename, 50)
	if err != nil || len(lines) == 0 {
		return false
	}

	for _, line := range lines {
		msg := strings.ToLower(line.Message)
		if strings.Contains(msg, "udp server listening") || strings.Contains(msg, "node "+strings.ToLower(nodeID)+" listening") {
			return true
		}
	}

	return false
}

func (s *State) recalculateLeaderFromRunningNodes() {
	running := make([]int, 0)

	for i := range s.nodes {
		if s.nodes[i].Status == "Running" {
			running = append(running, i)
		}
	}

	if len(running) == 0 {
		s.leader = ""
		return
	}

	if s.leader != "" {
		for _, idx := range running {
			if s.nodes[idx].ID == s.leader {
				s.nodes[idx].Role = "Leader"
				for _, j := range running {
					if j != idx {
						s.nodes[j].Role = "Follower"
					}
				}
				return
			}
		}
	}

	order := []string{"node1", "node2", "node3"}
	for _, id := range order {
		for _, idx := range running {
			if s.nodes[idx].ID == id {
				s.leader = id
				s.nodes[idx].Role = "Leader"
				for _, j := range running {
					if j != idx {
						s.nodes[j].Role = "Follower"
					}
				}
				return
			}
		}
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
		if s.nodes[i].Status != "Running" {
			continue
		}

		switch s.nodes[i].ID {
		case s.leader:
			s.nodes[i].LastAction = leaderAction
			s.nodes[i].Role = "Leader"
			s.nodes[i].CurrentTerm = s.currentTerm
		default:
			s.nodes[i].Role = "Follower"
			s.nodes[i].LastAction = followerAction
			s.nodes[i].CurrentTerm = s.currentTerm
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
					if s.nodes[j].ID != id && s.nodes[j].Status == "Running" {
						s.nodes[j].CurrentTerm = s.currentTerm
						s.nodes[j].Role = "Follower"
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
