package main

import (
	"bufio"
	"encoding/hex"
	"flag"
	"fmt"
	"github.com/sirupsen/logrus"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"vpn/client"
	"vpn/config"
)

func main() {
	var (
		serverAddr = flag.String("server", "localhost:9999", "VPN server address")
		clientIP   = flag.String("ip", "10.0.0.2", "Client VPN IP")
		dns        = flag.String("dns", "8.8.8.8,8.8.4.4", "DNS server (comma separated)")
		mtu        = flag.Int("mtu", 1400, "MTU size")
		loglevel   = flag.String("log", "info", "Log level (debug, info, warn, error)")
		key        = flag.String("key", "", "Shared key (hex encoded)")
		stats      = flag.Bool("stats", false, "Show statistics")
	)
	flag.Parse()
	level, err := logrus.ParseLevel(*loglevel)
	if err != nil {
		logrus.Fatalf("Invalid log level: %s", err)
	}
	logrus.SetLevel(level)
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})
	if os.Getegid() != 0 {
		logrus.Fatal("This program must be run as root")
	}
	cfg := config.NewClientConfig(*serverAddr)
	cfg.ClientIP = *clientIP
	cfg.MTU = *mtu
	if *dns != "" {
		cfg.DNS = strings.Split(*dns, ",")
	}

	if *key != "" {
		keyBytes, err := hex.DecodeString(*key)
		if err != nil {
			logrus.Fatalf("Failed to decode key: %s", err)
		}
		if len(keyBytes) != 32 {
			logrus.Fatalf("Invalid key size: %d must be 32 bytes", len(keyBytes))
		}
		cfg.SharedKey = keyBytes
	} else {
		fmt.Print("Enter shared key: ")
		reader := bufio.NewReader(os.Stdin)
		keyStr, _ := reader.ReadString('\n')
		keyStr = strings.TrimSpace(keyStr)
		keyBytes, err := hex.DecodeString(keyStr)
		if err != nil {
			logrus.Fatalf("Failed to decode key: %s", err)
		}
		if len(keyBytes) != 32 {
			logrus.Fatalf("Invalid key size: %d must be 32 bytes", len(keyBytes))
		}
		cfg.SharedKey = keyBytes
	}

	logrus.Info("Starting VPN client")
	logrus.Infof("Configuration:")
	logrus.Infof("  Server: %s", cfg.ServerAddr)
	logrus.Infof("  Client IP: %s", cfg.ClientIP)
	logrus.Infof("  DNS: %v", cfg.DNS)
	logrus.Infof("  MTU: %d", cfg.MTU)

	vpnClient, err := client.NewClient(cfg)
	if err != nil {
		logrus.Fatalf("Failed to create VPN client: %s", err)
	}
	if err := vpnClient.Connect(); err != nil {
		logrus.Fatalf("Failed to connect to VPN: %s", err)
	}
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	if *stats {
		go showStats(vpnClient)
	}
	sig := <-sigChan
	logrus.Infof("Received signal: %s, disconnecting", sig)
	if err := vpnClient.Disconnect(); err != nil {
		logrus.Fatalf("Failed to disconnect: %s", err)
	}
	logrus.Info("VPN client disconnected")
}

func showStats(client *client.Client) {
	ticker := time.NewTicker(time.Second * 5)
	defer ticker.Stop()
	for range ticker.C {
		bytesIn, bytesOut := client.GetStats()
		logrus.Infof("Statistics: IN: %s, OUT: %s",
			formatBytes(bytesIn), formatBytes(bytesOut))
	}
}

func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
func innit() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "GoVPN Client v1.0\n\n")
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "  sudo %s -server vpn.example.com:9999 -key <shared-key>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nNote: The shared key must match the server's key\n")
	}
}
