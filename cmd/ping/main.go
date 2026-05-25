package main

import (
	"errors"
	"fmt"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/protocol/chainsync"
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	Magic      uint32
	SocketPath string `split_words:"true"`
	Address    string
	Network    string
}

type NodeConnection interface {
	Dial(network, address string) error
	ChainSync() *chainsync.ChainSync
	Close() error
}

type PingResult struct {
	ConnectionTime time.Duration
	ProtocolTime   time.Duration
	Error          error
}

func GetConnectionDetails(cfg *Config) (string, string, bool) {
	// Prefer TCP address if both are provided
	if cfg.Address != "" {
		return "tcp", cfg.Address, true
	}
	if cfg.SocketPath != "" {
		return "unix", cfg.SocketPath, false
	}
	return "", "", false
}

func NewConnection(cfg *Config) (NodeConnection, error) {
	if cfg.Magic == 0 {
		network, ok := ouroboros.NetworkByName(cfg.Network)
		if !ok {
			return nil, fmt.Errorf("invalid network specified: %v", cfg.Network)
		}
		cfg.Magic = network.NetworkMagic
	}

	errorChan := make(chan error)
	go func() {
		for err := range errorChan {
			fmt.Printf("connection error: %v\n", err)
		}
	}()

	_, _, isTcp := GetConnectionDetails(cfg)

	return ouroboros.NewConnection(
		ouroboros.WithNetworkMagic(cfg.Magic),
		ouroboros.WithNodeToNode(isTcp),
		ouroboros.WithKeepAlive(true),
		ouroboros.WithErrorChan(errorChan),
	)
}

var getTip = func(sync *chainsync.ChainSync) (*chainsync.Tip, error) {
	return sync.Client.GetCurrentTip()
}

func PingNode(conn NodeConnection, cfg *Config) PingResult {
	network, address, ok := GetConnectionDetails(cfg)
	if !ok {
		return PingResult{
			Error: errors.New(
				"no connection details provided, must specify either socket path or TCP address",
			),
		}
	}

	start := time.Now()
	if err := conn.Dial(network, address); err != nil {
		return PingResult{Error: fmt.Errorf("connection failed: %w", err)}
	}
	connTime := time.Since(start)

	start = time.Now()

	if _, err := getTip(conn.ChainSync()); err != nil {
		return PingResult{
			ConnectionTime: connTime,
			Error:          fmt.Errorf("protocol error: %w", err),
		}
	}
	protoTime := time.Since(start)

	return PingResult{
		ConnectionTime: connTime,
		ProtocolTime:   protoTime,
	}
}

func main() {
	var cfg Config
	if err := envconfig.Process("cardano_node", &cfg); err != nil {
		fmt.Printf("Config error: %v\n", err)
		return
	}

	conn, err := NewConnection(&cfg)
	if err != nil {
		fmt.Printf("Connection error: %v\n", err)
		return
	}
	defer conn.Close()

	result := PingNode(conn, &cfg)
	if result.Error != nil {
		fmt.Printf("Ping failed: %v\n", result.Error)
		return
	}

	connType := "Node-to-Node"
	if cfg.SocketPath != "" {
		connType = "UNIX socket"
	}
	fmt.Printf("%s Ping Results:\n", connType)
	fmt.Printf("Connection established in: %s\n", result.ConnectionTime)
	fmt.Printf("Protocol response time:   %s\n", result.ProtocolTime)
}
