# Fault-Tolerant Distributed Payment Processing System

## Team Members
| Name                                 | Registration Number |
|--------------------------------------|---------------------|
| Member 1 - Wickramasinghe G. K. D. K | IT24101516          |       
| Member 2 - Wijesinghe K              | IT24102587          |       
| Member 3 - Yavindi M. D. C           | IT24101636          |       
| Member 4 - Perera K. T. L            | IT24610793          |       

## How to Run

### Prerequisites
- Go 1.21 or higher — https://go.dev/dl/

### Start the cluster (3 separate terminals)
```bash
go run cmd/node/main.go --id node1 --port 8001
go run cmd/node/main.go --id node2 --port 8002
go run cmd/node/main.go --id node3 --port 8003
```

### Run all tests
```bash
go test ./... -race
```

### Open the UI
Navigate to http://127.0.0.1:8001 in your browser.

## Module Ownership
- Member 1 — internal/fault/
- Member 2 — internal/replication/
- Member 3 — internal/clock/
- Member 4 — internal/consensus/