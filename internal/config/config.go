package config

type NodeConfig struct {
	ID   string
	Host string
	Port int
}

// Config stores all important system configuration
type Config struct {
	Nodes             []NodeConfig
	ReplicationFactor int
	TimeSyncInterval  int
	MaxClockSkew      int
	HeartbeatInterval int
	HeartbeatTimeout  int
	RecoveryBatchSize int
}

// Load returns a hardcoded config for now
func Load() *Config {
	return &Config{
		ReplicationFactor: 3,
		TimeSyncInterval:  10,
		MaxClockSkew:      50,
		HeartbeatInterval: 2,
		HeartbeatTimeout:  5,
		RecoveryBatchSize: 100,
		Nodes: []NodeConfig{
			{ID: "node1", Host: "127.0.0.1", Port: 8001},
			{ID: "node2", Host: "127.0.0.1", Port: 8002},
			{ID: "node3", Host: "127.0.0.1", Port: 8003},
		},
	}
}

// PeersExcluding returns all other nodes except the current one
func (c *Config) PeersExcluding(id string) []NodeConfig {
	var peers []NodeConfig
	for _, n := range c.Nodes {
		if n.ID != id {
			peers = append(peers, n)
		}
	}
	return peers
}

// FindNode returns a node config by ID
func (c *Config) FindNode(id string) (NodeConfig, bool) {
	for _, n := range c.Nodes {
		if n.ID == id {
			return n, true
		}
	}
	return NodeConfig{}, false
}
