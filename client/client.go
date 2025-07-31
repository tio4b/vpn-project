package client

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
	config      *config.Config
	conn        net.Conn
	tun         *network.TUNInterface
	routeManger *network.RouteManager
	cipher      *crypto.Cipher

	bytesIn  uint64
	bytesOut uint64

	stopChan chan struct{}
	wg       sync.WaitGroup
	mu       sync.Mutex
}

func NewClient(config *config.Config) (*Client, error) {
	return &Client{
		config:   config,
		stopChan: make(chan struct{}),
	}, nil
}

func (client *Client) Connect() error {
	logrus.Infof("Connecting to VPN server %s", client.config.ServerAddr)
	tlsConfig := crypto.NewClientTSLConfig(true)
	conn, err := tls.Dial("tcp", client.config.ServerAddr, tlsConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %v", err)
	}
	client.conn = conn
	cipher, err := crypto.NewCipher(client.config.SharedKey)
	if err != nil {
		return fmt.Errorf("failed to create cipher: %v", err)
	}
	client.cipher = cipher
	handshakeMessage := protocol.CreateHandshake(protocol.TypeHandshake, client.config.ClientIP, client.config.SharedKey)
	if err := protocol.WriteMessage(conn, handshakeMessage); err != nil {
		return fmt.Errorf("failed to write handshake message: %v", err)
	}
	message, err := protocol.ReadMessage(conn)
	if err != nil {
		return fmt.Errorf("failed to read handshake response: %v", err)
	}
	if message.Header.Type != protocol.TypeHandshakeAck {
		return fmt.Errorf("expected hanshakeAck, but got: %v", message.Header.Type)
	}
	logrus.Info("Successfully authenticated with server")
	tun, err := network.NewTUNInterface(client.config.ClientIP, client.config.VPNSubnet, client.config.MTU, false)
	if err != nil {
		return fmt.Errorf("failed to create tun interface: %v", err)
	}
	client.tun = tun
	logrus.Infof("TUN interface %s created with IP %s", tun.Name(), client.config.ClientIP)
	client.routeManger = network.NewRouteManager(tun.Name(), client.config.ServerAddr, client.config.DNS)
	if err := client.routeManger.SetupClientRoutes(); err != nil {
		return fmt.Errorf("failed to setup client routes: %v", err)
	}

	client.wg.Add(3)
	go client.tunReader()
	go client.serverReader()
	go client.keepAlive()
	return nil
}

func (client *Client) tunReader() {
	defer client.wg.Done()
	buffer := make([]byte, client.config.MTU+14)
	for {
		select {
		case <-client.stopChan:
			return
		default:
			n, err := client.tun.Read(buffer)
			if err != nil {
				logrus.Errorf("Failed to read from tun interface: %v", err)
			}
			packet, err := protocol.ParseIPPacket(buffer[:n])
			if err == nil {
				logrus.Debugf("Read %s packet from TUN: %s to %s (%d bytes)",
					packet.ProtocolName(), packet.SrcIp, packet.DstIp, n)
			}
			ciphertext, err := client.cipher.Encrypt(buffer[:n])
			if err != nil {
				logrus.Errorf("Failed to encrypt packet: %v", err)
				continue
			}
			message := protocol.NewMessage(protocol.TypeData, ciphertext)
			client.mu.Lock()
			err = protocol.WriteMessage(client.conn, message)
			if err == nil {
				client.bytesOut += uint64(n)
			}
			client.mu.Unlock()
			if err != nil {
				logrus.Errorf("Failed to send data to server: %v", err)
				return
			}
		}
	}
}

func (client *Client) serverReader() {
	defer client.wg.Done()
	for {
		select {
		case <-client.stopChan:
			return
		default:
			message, err := protocol.ReadMessage(client.conn)
			if err != nil {
				logrus.Errorf("Failed to read from server: %v", err)
				return
			}
			switch message.Header.Type {
			case protocol.TypeData:
				plaintext, err := client.cipher.Decrypt(message.Data)
				if err != nil {
					logrus.Errorf("Failed to decrypt data: %v", err)
					continue
				}
				packet, err := protocol.ParseIPPacket(plaintext)
				if err == nil {
					logrus.Debugf("Received %s packet from server: %s to %s (%d bytes)",
						packet.ProtocolName(), packet.SrcIp, packet.DstIp, len(plaintext))
				}
				if _, err := client.conn.Write(plaintext); err != nil {
					logrus.Errorf("Failed to write to TUN: %v", err)
				}
				client.mu.Lock()
				client.bytesIn += uint64(len(plaintext))
				client.mu.Unlock()

			case protocol.TypeKeepAlive:
				logrus.Debug("Received keep-alive from server")
			case protocol.TypeDisconnect:
				logrus.Info("Server requested disconnect")
				return
			}
		}
	}
}

func (client *Client) keepAlive() {
	defer client.wg.Done()
	ticker := time.NewTicker(client.config.KeepAlive)
	defer ticker.Stop()
	for {
		select {
		case <-client.stopChan:
			return
		case <-ticker.C:
			message := protocol.NewMessage(protocol.TypeKeepAlive, nil)
			client.mu.Lock()
			err := protocol.WriteMessage(client.conn, message)
			client.mu.Unlock()
			if err != nil {
				logrus.Errorf("Failed to send keepalive: %v", err)
				return
			}
		}
	}
}

func (client *Client) GetStats() (bytesIn, bytesOut uint64) {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.bytesIn, client.bytesOut
}

func (client *Client) Disconnect() error {
	logrus.Info("Disconnecting from VPN server")
	close(client.stopChan)
	if client.conn != nil {
		message := protocol.NewMessage(protocol.TypeDisconnect, nil)
		protocol.WriteMessage(client.conn, message)
		client.conn.Close()
	}
	client.wg.Wait()
	if client.routeManger != nil {
		if err := client.routeManger.RestoreRoutes(); err != nil {
			logrus.Warnf("Failed to restore routes: %v", err)
		}
	}
	if client.tun != nil {
		client.tun.Close()
	}
	logrus.Info("Successfully disconnected from VPN server")
	return nil
}
