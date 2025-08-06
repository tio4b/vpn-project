#!/bin/bash

set -e

echo "VPN Linux Setup Script"
echo "========================"

if [ "$EUID" -ne 0 ]; then
    echo "Please run as root (use sudo)"
    exit 1
fi

check_command() {
    if ! command -v $1 &> /dev/null; then
        echo "FAIL $1 is not installed"
        return 1
    else
        echo "OK $1 is installed"
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

cat > /etc/systemd/system/vpn-server.service << EOF
[Unit]
Description=VPN Server
After=network.target

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/vpn-server -listen :9999 -ip 10.0.0.1
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

cat > /etc/systemd/system/vpn-client.service << EOF
[Unit]
Description=VPN Client
After=network.target

[Service]
Type=simple
User=root
ExecStart=/usr/local/bin/vpn-client -server SERVER_ADDRESS:9999 -key SHARED_KEY
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload

echo -e "\nSetup completed!"
echo "=================="
echo ""
echo "To build and install VPN:"
echo "  cd /path/to/vpn"
echo "  go build -o vpn-server ./main/server"
echo "  go build -o vpn-client ./main/client"
echo "  sudo cp vpn-server vpn-client /usr/local/bin/"
echo ""
echo "To run server:"
echo "  sudo vpn-server -listen :9999"
echo ""
echo "To run client:"
echo "  sudo vpn-client -server <server-ip>:9999 -key <shared-key>"
echo ""
echo "To use systemd services:"
echo "  Edit /etc/systemd/system/vpn-client.service with your server details"
echo "  sudo systemctl start vpn-server"
echo "  sudo systemctl enable vpn-server"
echo ""
