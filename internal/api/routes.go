package api

import "net/http"

func RegisterRoutes(mux *http.ServeMux, h *Handler) {
	mux.HandleFunc("/api/dashboard", h.GetDashboard)
	mux.HandleFunc("/api/leader", h.GetLeader)
	mux.HandleFunc("/api/nodes", h.GetNodes)
	mux.HandleFunc("/api/payments", h.GetPayments)
	mux.HandleFunc("/api/logs", h.GetLogs)
	mux.HandleFunc("/api/fail-node", h.FailNode)
	mux.HandleFunc("/api/rejoin-node", h.RejoinNode)
}