package config

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

type Config struct {
	Mode string // "server" "client"
	Log  string // "debug" "info" "warn" "error"
	MTU  int

	ServerAddr string
	ListenAddr string
	TunName    string

	ServerIP  string
	ClientIP  string
	VPNSubnet string
	DNS       []string

	TLSCert   string
	TLSKey    string
	SharedKey []byte

	KeepAlive time.Duration
	Timeout   time.Duration
}

func newConfig() *Config {
	key := make([]byte, 32)
	rand.Read(key)
	return &Config{
		Mode:       "client",
		Log:        "info",
		MTU:        1400,
		ServerAddr: "localhost:9999",
		ListenAddr: ":9999",
		TunName:    "tun",
		ServerIP:   "10.0.0.1",
		ClientIP:   "10.0.0.2",
		VPNSubnet:  "10.0.0.0/24",
		DNS:        []string{"8.8.8.8", "8.8.4.4"},
		SharedKey:  key,
		KeepAlive:  30 * time.Second,
		Timeout:    60 * time.Second,
	}
}

func NewServerConfig() *Config {
	cfg := newConfig()
	cfg.Mode = "server"
	return cfg
}

func NewClientConfig(serverAddr string) *Config {
	cfg := newConfig()
	cfg.Mode = "client"
	cfg.ServerAddr = serverAddr
	return cfg
}

func (c *Config) KeyString() string {
	return hex.EncodeToString(c.SharedKey)
}
