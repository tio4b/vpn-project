# VPN
## Architecture

```
┌─────────────┐                    ┌─────────────┐
│   Client    │                    │   Server    │
│             │                    │             │
│  TUN Interface <──Encrypted──> TUN Interface   │
│  10.0.0.2   │     Tunnel         │  10.0.0.1   │
│             │                    │             │
│  App → TUN → Encrypt → Send → Decrypt → TUN → Internet
└─────────────┘                    └─────────────┘
```

## Installation

### Prerequisites

- Go 1.19 or higher
- Linux, macOS, or Windows
- Root

### Build

```bash
git clone https://github.com/yourusername/govpn.git
cd govpn


go mod init vpn
go get github.com/songgao/water
go get github.com/sirupsen/logrus


go build -o vpn-server ./main/server
go build -o vpn-client ./main/client


sudo cp govpn-server vpn-client /usr/local/bin/
```

### Quick Setup

```bash
sudo ./scripts/setup_linux.sh
```


### Server

Start the VPN server:

```bash
sudo ./vpn-server -listen :9999 -ip 10.0.0.1
```

The server will display the shared key on startup:

```
Starting VPN Server
Configuration:
 Listen address: :9999
 Server IP: 10.0.0.1
 VPN Subnet: 10.0.0.0/24
 Shared Key: ...
```

### Client

Connect to the VPN server:

```bash
sudo ./vpn-client -server <server-ip>:9999 -key <shared-key>
```

Or enter the key interactively:

```bash
sudo .vpn-client -server vpn.example.com:9999
Enter shared key (hex): a1b2c3d4e5f6...
```

## Command Line Options

### Server Options

| Option | Default | Description |
|--------|---------|-------------|
| `-listen` | `:9999` | Listen address and port |
| `-ip` | `10.0.0.1` | Server VPN IP address |
| `-subnet` | `10.0.0.0/24` | VPN subnet |
| `-mtu` | `1400` | MTU size |
| `-log` | `info` | Log level (debug, info, warn, error) |

### Client Options

| Option | Default | Description |
|--------|---------|-------------|
| `-server` | `localhost:9999` | VPN server address |
| `-ip` | `10.0.0.2` | Client VPN IP address |
| `-dns` | `8.8.8.8,8.8.4.4` | DNS servers (comma separated) |
| `-mtu` | `1400` | MTU size |
| `-key` | - | Shared key (hex encoded) |
| `-stats` | `false` | Show traffic statistics |
| `-log` | `info` | Log level |

## Configuration Examples

### Basic Setup

**Server:**
```bash
sudo ./vpn-server
```

**Client:**
```bash
sudo .vpn-client -server 192.168.1.100:9999 -key <key-from-server>
```

### Custom Network

**Server:**
```bash
sudo ./vpn-server -ip 172.16.0.1 -subnet 172.16.0.0/24 -listen :8443
```

**Client:**
```bash
sudo ./vpn-client -server vpn.company.com:8443 -ip 172.16.0.10
```

### Debug Mode with Statistics

```bash
sudo ./vpn-client -server vpn.example.com:9999 -log debug -stats
```



