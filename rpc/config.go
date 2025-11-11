package rpc

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type RPCConfig struct {
	Username string `json:"username"`
	Password string `json:"password"`
	WSURL    string `json:"ws_url"`
}

const rpcConfigFile = ".unrealircd_rpc_config"

func LoadRPCConfig() (*RPCConfig, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	configPath := filepath.Join(home, rpcConfigFile)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, nil // No config file
	}
	file, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var config RPCConfig
	err = json.NewDecoder(file).Decode(&config)
	return &config, err
}

func SaveRPCConfig(config *RPCConfig) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	configPath := filepath.Join(home, rpcConfigFile)
	file, err := os.Create(configPath)
	if err != nil {
		return err
	}
	defer file.Close()
	return json.NewEncoder(file).Encode(config)
}

func TestRPCConnection(config *RPCConfig) error {
	// Test actual RPC connection
	if config.Username == "" || config.Password == "" || config.WSURL == "" {
		return fmt.Errorf("missing RPC configuration")
	}

	client, err := NewRPCClient(config)
	if err != nil {
		return fmt.Errorf("failed to create RPC client: %w", err)
	}
	defer client.Close()

	if err := client.Connect(); err != nil {
		return fmt.Errorf("failed to connect to RPC server: %w", err)
	}

	// Actually test by trying to get server info
	_, err = client.conn.Rpc().Info()
	if err != nil {
		return fmt.Errorf("failed to get server info: %w", err)
	}

	return nil
}
