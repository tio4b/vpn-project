package main

import (
	"flag"
	"fmt"
	"github.com/sirupsen/logrus"
	"os"
	"os/signal"
	"syscall"
	"vpn/config"
	"vpn/server"
)

func main() {
	var (
		listenAddr = flag.String("listen", ":9999", "Listen address")
		serverIP   = flag.String("ip", "10.0.0.1", "Server VPN IP")
		subnet     = flag.String("subnet", "10.0.0.0/24", "VPN subnet")
		mtu        = flag.Int("mtu", 1400, "MTU size")
		logLevel   = flag.String("log", "info", "Log level (debug, info, warn, error)")
		keyFile    = flag.String("key", "", "Shared key file (if not specified, generates random)")
	)
	flag.Parse()
	level, err := logrus.ParseLevel(*logLevel)
	if err != nil {
		logrus.Fatalf("Invalid log level: %s", *logLevel)
	}
	logrus.SetLevel(level)
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})
	if os.Getegid() != 0 {
		logrus.Fatal("This program must be run as root")
	}
	cfg := config.NewServerConfig()
	cfg.ListenAddr = *listenAddr
	cfg.ServerIP = *serverIP
	cfg.VPNSubnet = *subnet
	cfg.MTU = *mtu
	if *keyFile != "" {
		logrus.Warn("Key file loading not implemented yet, using random key")
	}
	logrus.Info("Starting GoVPN Server")
	logrus.Infof("Configuration:")
	logrus.Infof("  Listen address: %s", cfg.ListenAddr)
	logrus.Infof("  Server IP: %s", cfg.ServerIP)
	logrus.Infof("  VPN Subnet: %s", cfg.VPNSubnet)
	logrus.Infof("  MTU: %d", cfg.MTU)
	logrus.Infof("  Shared Key: %s", cfg.KeyString())

	server, err := server.NewServer(cfg)
	if err != nil {
		logrus.Fatalf("Failed to create server: %v", err)
	}
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		if err := server.Start(); err != nil {
			logrus.Fatalf("Failed to start server: %v", err)
		}
	}()
	sig := <-sigChan
	logrus.Infof("Received signal %v, shutting down", sig)
	if err := server.Stop(); err != nil {
		logrus.Fatalf("Failed to stop server: %v", err)
	}
	logrus.Info("Server stopped")
}

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "GoVPN Server v1.0\n\n")
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "  sudo %s -listen :9999 -ip 10.0.0.1 -subnet 10.0.0.0/24\n", os.Args[0])
	}
}
