#!/bin/bash

set -e

echo "GoVPN Linux Setup Script"
echo "========================"

if [ "$EUID" -ne 0 ]; then
    echo "Please run as root (use sudo)"
    exit 1
fi

check_command() {
    if ! command -v $1 &> /dev/null; then
        echo "X $1 is not installed"
        return 1
    else
        echo "V $1 is installed"
        return 0
    fi
}

echo -e "\nChecking dependencies..."
DEPS_OK=true

check_command ip || DEPS_OK=false
check_command iptables || DEPS_OK=false
check_command sysctl || DEPS_OK=false

if [ "$DEPS_OK" = false ]; then
    echo -e "\nInstalling missing dependencies..."
    apt-get update
    apt-get install -y iproute2 iptables procps
fi

echo -e "\nEnabling IP forwarding..."
sysctl -w net.ipv4.ip_forward=1
echo "net.ipv4.ip_forward=1" >> /etc/sysctl.conf

echo -e "\nConfiguring firewall..."

iptables -P FORWARD ACCEPT

iptables -A INPUT -i tun+ -j ACCEPT
iptables -A FORWARD -i tun+ -j ACCEPT
iptables -A OUTPUT -o tun+ -j ACCEPT

if command -v netfilter-persistent &> /dev/null; then
    netfilter-persistent save
elif command -v iptables-save &> /dev/null; then
    iptables-save > /etc/iptables/rules.v4
fi

echo -e "\nCreating systemd service..."

cat > /etc/systemd/system/govpn-server.service << EOF
[Unit]
Description=GoVPN Server
After=network.target

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/govpn-server -listen :9999 -ip 10.0.0.1
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

cat > /etc/systemd/system/govpn-client.service << EOF
[Unit]
Description=GoVPN Client
After=network.target

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/govpn-client -server SERVER_ADDRESS:9999 -key SHARED_KEY
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload

echo -e "\nSetup completed!"
echo "=================="
echo ""
echo "To build and install GoVPN:"
echo "  cd /path/to/govpn"
echo "  go build -o govpn-server ./cmd/server"
echo "  go build -o govpn-client ./cmd/client"
echo "  sudo cp govpn-server govpn-client /usr/local/bin/"
echo ""
echo "To run server:"
echo "  sudo govpn-server -listen :9999"
echo ""
echo "To run client:"
echo "  sudo govpn-client -server <server-ip>:9999 -key <shared-key>"
echo ""
echo "To use systemd services:"
echo "  Edit /etc/systemd/system/govpn-client.service with your server details"
echo "  sudo systemctl start govpn-server"
echo "  sudo systemctl enable govpn-server"
echo ""