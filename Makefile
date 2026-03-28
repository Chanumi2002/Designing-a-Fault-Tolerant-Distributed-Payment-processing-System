.PHONY: test run-cluster

test:
	go test ./... -race

run-cluster:
	go run cmd/node/main.go --id node1 --port 8001 &
	go run cmd/node/main.go --id node2 --port 8002 &
	go run cmd/node/main.go --id node3 --port 8003 &