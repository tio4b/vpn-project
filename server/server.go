package server

import (
	"crypto/tls"
	"fmt"
	"github.com/sirupsen/logrus"
	"net"
	"sync"
	"time"
	"vpn/config"
	"vpn/crypto"
	"vpn/network"
	"vpn/protocol"
)

type Client struct {
	ID       string
	Conn     net.Conn
	IP       string
	Cipher   *crypto.Cipher
	LastSeen time.Time
	mu       sync.Mutex
}

type Server struct {
	config    *config.Config
	tun       *network.TUNInterface
	clients   map[string]*Client
	clientsMu sync.RWMutex
	listener  net.Listener

	tunChan  chan []byte
	stopChan chan struct{}
}

func NewServer(config *config.Config) (*Server, error) {
	return &Server{
		config:   config,
		clients:  make(map[string]*Client),
		tunChan:  make(chan []byte, 100),
		stopChan: make(chan struct{}),
	}, nil
}

func (server *Server) Start() error {
	tun, err := network.NewTUNInterface(server.config.ServerIP, server.config.VPNSubnet, server.config.MTU, true)
	if err != nil {
		return fmt.Errorf("new tun interface: %v", err)
	}
	server.tun = tun
	logrus.Infof("new tun interface %s with IP %s ", tun.Name(), server.config.ServerIP)
	tlsConfig, err := crypto.NewServerTSLConfig()
	if err != nil {
		return fmt.Errorf("create server tls config: %v", err)
	}
	listener, err := tls.Listen("tcp", server.config.ListenAddr, tlsConfig)
	if err != nil {
		return fmt.Errorf("create server listener: %v", err)
	}
	server.listener = listener
	go server.tunReader()
	go server.clientCleaner()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-server.stopChan:
				return nil
			default:
				logrus.Errorf("failed to accept new connection: %v", err)
				continue
			}
		}

		go server.handleConnection(conn)
	}
}

func (server *Server) handleConnection(conn net.Conn) {
	defer conn.Close()
	clientAddr := conn.RemoteAddr().String()
	logrus.Infof("new client connection from %s", clientAddr)
	message, err := protocol.ReadMessage(conn)
	if err != nil {
		logrus.Errorf("failed to read handshake: %v", err)
		return
	}
	if message.Header.Type != protocol.TypeHandshake {
		logrus.Errorf("expected handshake but got: %v", message.Header.Type)
	}
	handshake, err := protocol.ParseHandshake(message.Data)
	if err != nil {
		logrus.Errorf("failed to parse handshake: %v", err)
		return
	}
	cipher, err := crypto.NewCipher(server.config.SharedKey)
	if err != nil {
		logrus.Errorf("failed to create cipher: %v", err)
		return
	}
	client := &Client{
		ID:       clientAddr,
		Conn:     conn,
		IP:       handshake.ClientIP,
		Cipher:   cipher,
		LastSeen: time.Now(),
	}

	server.clientsMu.Lock()
	server.clients[client.ID] = client
	server.clientsMu.Unlock()

	ackMessage := protocol.NewMessage(protocol.TypeHandshakeAck, []byte("OK"))
	if err := protocol.WriteMessage(conn, ackMessage); err != nil {
		logrus.Errorf("failed to send ack message: %v", err)
		return
	}
	logrus.Infof("Client %s authenticated with IP %s", clientAddr, handshake.ClientIP)
	for {
		message, err := protocol.ReadMessage(conn)
		if err != nil {
			logrus.Errorf("failed to read message: %v", err)
			break
		}
		client.mu.Lock()
		client.LastSeen = time.Now()
		client.mu.Unlock()

		switch message.Header.Type {
		case protocol.TypeData:
			plaintext, err := cipher.Decrypt(message.Data)
			if err != nil {
				logrus.Errorf("failed to decrypt message: %v", err)
				continue
			}
			packet, err := protocol.ParseIPPacket(plaintext)
			if err != nil {
				logrus.Errorf("failed to parse packet: %v", err)
				continue
			}
			logrus.Debugf("Received %s packet from %s to %s (%d bytes)",
				packet.ProtocolName(), packet.SrcIp, packet.DstIp, len(plaintext))
			if _, err := server.tun.Write(plaintext); err != nil {
				logrus.Errorf("failed to write to TUN: %v", err)
				continue
			}
		case protocol.TypeKeepAlive:
			keepAliveMessage := protocol.NewMessage(protocol.TypeKeepAlive, nil)
			if err := protocol.WriteMessage(conn, keepAliveMessage); err != nil {
				logrus.Errorf("failed to write keep alive: %v", err)
				continue
			}
		case protocol.TypeDisconnect:
			logrus.Infof("Client %s disconnected", clientAddr)
			return
		}
	}
	server.clientsMu.Lock()
	delete(server.clients, client.ID)
	server.clientsMu.Unlock()

	logrus.Infof("Client %s removed", clientAddr)
}

func (server *Server) tunReader() {
	buffer := make([]byte, server.config.MTU+14)
	for {
		select {
		case <-server.stopChan:
			return
		default:
			n, err := server.tun.Read(buffer)
			if err != nil {
				logrus.Errorf("tun read error: %v", err)
				continue
			}
			packet, err := protocol.ParseIPPacket(buffer[:n])
			if err != nil {
				logrus.Errorf("parse packet error: %v", err)
				continue
			}
			logrus.Debugf("Read %s packet from TUN: %s to %s (%d bytes)",
				packet.ProtocolName(), packet.SrcIp, packet.DstIp, n)
			server.clientsMu.Lock()
			var targetClient *Client
			for _, client := range server.clients {
				if client.IP == packet.DstIp.String() {
					targetClient = client
					break
				}
			}
			server.clientsMu.Unlock()
			if targetClient == nil {
				logrus.Debugf("No client found for IP %s", packet.DstIp)
				continue
			}
			ciphertext, err := targetClient.Cipher.Encrypt(buffer[:n])
			if err != nil {
				logrus.Errorf("cipher encrypt error: %v", err)
				continue
			}
			message := protocol.NewMessage(protocol.TypeData, ciphertext)
			targetClient.mu.Lock()
			err = protocol.WriteMessage(targetClient.Conn, message)
			targetClient.mu.Unlock()
			if err != nil {
				logrus.Errorf("write message error: %v", err)
			}
		}
	}
}

func (server *Server) clientCleaner() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-server.stopChan:
			return
		case <-ticker.C:
			now := time.Now()
			server.clientsMu.Lock()
			for _, client := range server.clients {
				client.mu.Lock()
				if now.Sub(client.LastSeen) > server.config.Timeout {
					logrus.Infof("Removing client %s", client.ID)
					client.Conn.Close()
					delete(server.clients, client.ID)
				}
				client.mu.Unlock()
			}
			server.clientsMu.Unlock()
		}
	}
}

func (server *Server) Stop() error {
	close(server.stopChan)
	if server.listener != nil {
		server.listener.Close()
	}
	if server.tun != nil {
		server.tun.Close()
	}
	server.clientsMu.Lock()
	for _, client := range server.clients {
		client.Conn.Close()
	}
	server.clientsMu.Unlock()
	return nil
}
