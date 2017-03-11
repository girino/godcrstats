package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"github.com/decred/dcrutil"
)

type ConnectionConfig struct {
	Host      string
	Port      int
	User      string
	Password  string
	CertPath  string
	IsTestnet bool
}

type WalletConnectionConfig ConnectionConfig
type DaemonConnectionConfig ConnectionConfig

type Config struct {
	WalletEnabled    bool
	WalletConfig     *WalletConnectionConfig
	DaemonConfig     *DaemonConnectionConfig
	HttpServerListen string
}

// global instance
var config *Config

func NewDefaultConfig() *Config {
	pathd := filepath.Join(dcrutil.AppDataDir("dcrd", false), "rpc.cert")
	pathw := filepath.Join(dcrutil.AppDataDir("dcrwallet", false), "rpc.cert")
	return &Config{
		WalletEnabled: false,
		WalletConfig: &WalletConnectionConfig{
			Host:      "localhost",
			Port:      0,
			User:      "rpcuser",
			Password:  "rpcpassword",
			CertPath:  pathw,
			IsTestnet: true,
		},
		DaemonConfig: &DaemonConnectionConfig{
			Host:      "localhost",
			Port:      0,
			User:      "rpcuser",
			Password:  "rpcpassword",
			CertPath:  pathd,
			IsTestnet: true,
		},
		HttpServerListen: ":8080",
	}
}

func (c *ConnectionConfig) CheckAndUpdatePort(isWallet bool) {
	if c.Port <= 0 {
		c.Port = 9109
		name := "dcrd"
		net := "mainnet"
		if isWallet {
			c.Port = 9110
			name = "dcrwallet"
		}
		if c.IsTestnet {
			c.Port = c.Port + 10000
			net = "testnet"
		}
		log.Printf("Port is missing or invalid. Using default for %s on %s: %d", name, net, c.Port)
	}
}

func (c *DaemonConnectionConfig) CheckAndUpdatePort() {
	((*ConnectionConfig)(c)).CheckAndUpdatePort(false)
}
func (c *WalletConnectionConfig) CheckAndUpdatePort() {
	((*ConnectionConfig)(c)).CheckAndUpdatePort(true)
}

func NewConfigFromFile(filename string) *Config {
	file, _ := os.Open(filename)
	decoder := json.NewDecoder(file)
	configuration := NewDefaultConfig()
	err := decoder.Decode(configuration)
	if err != nil {
		log.Fatalf("Error loading %s: %v", filename, err)
	}

	if configuration.WalletEnabled {
		configuration.WalletConfig.CheckAndUpdatePort()
	}
	configuration.DaemonConfig.CheckAndUpdatePort()

	return configuration
}
