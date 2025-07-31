package network

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/songgao/water"
	"net"
	"os/exec"
	"runtime"
)

type TUNInterface struct {
	iface    *water.Interface
	name     string
	ip       string
	subnet   string
	mtu      int
	isServer bool
}

func NewTUNInterface(ip, subnet string, mtu int, isServer bool) (*TUNInterface, error) {
	config := water.Config{
		DeviceType: water.TUN,
	}
	iface, err := water.New(config)
	if err != nil {
		return nil, fmt.Errorf("create tun interface: %v", err)
	}
	tun := &TUNInterface{
		iface:    iface,
		name:     iface.Name(),
		ip:       ip,
		subnet:   subnet,
		mtu:      mtu,
		isServer: isServer,
	}

	if err := tun.Configure(); err != nil {
		return nil, fmt.Errorf("configure tun interface: %v", err)
	}

	return tun, nil
}

func (tun *TUNInterface) Configure() error {
	switch runtime.GOOS {
	case "linux":
		return tun.configureLinux()
	case "darwin":
		return tun.configureDarwin()
	case "windows":
		return tun.configureWindows()
	default:
		return fmt.Errorf("unsupported platform")
	}
}

func (tun *TUNInterface) configureLinux() error {
	cmd := exec.Command("ip", "addr", "add", tun.ip+"/24", "dev", tun.name)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set up IP address: %v", err)
	}
	cmd = exec.Command("ip", "link", "set", "dev", tun.name, "up")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to bring up interface: %v", err)
	}
	cmd = exec.Command("ip", "link", "set", "dev", tun.name, "mtu", fmt.Sprintf("%d", tun.mtu))
	if err := cmd.Run(); err != nil {
		logrus.Warnf("failed to set mtu: %v", err)
	}

	if tun.isServer {
		cmd = exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1")
		if err := cmd.Run(); err != nil {
			logrus.Warnf("failed to set ip forward: %v", err)
		}
		cmd = exec.Command("iptables", "-t", "nat", "POSTROUTING",
			"-s", tun.subnet, "-o", getDefaultInterface(), "-j", "MASQUERADE")
		if err := cmd.Run(); err != nil {
			logrus.Warnf("failed to set up iptables: %v", err)
		}
	}
	return nil
}
func (tun *TUNInterface) configureDarwin() error {
	cmd := exec.Command("ifconfig", tun.name, tun.ip, tun.ip)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to bring up interface: %v", err)
	}
	cmd = exec.Command("ifconfig", tun.name, "up")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to bring up interface: %v", err)
	}
	cmd = exec.Command("ifconfig", tun.name, "mtu", fmt.Sprintf("%d", tun.mtu))
	if err := cmd.Run(); err != nil {
		logrus.Warnf("failed to set mtu: %v", err)
	}
	if tun.isServer {
		cmd = exec.Command("sysctl", "-w", "net.inet.ip.forwarding=1")
		if err := cmd.Run(); err != nil {
			logrus.Warnf("failed to set ip forward: %v", err)
		}
	}
	return nil
}
func (tun *TUNInterface) configureWindows() error {
	cmd := exec.Command("netsh", "interface", "ip", "set", "address",
		fmt.Sprintf("name=%s", tun.name), "static", tun.ip, "255.255.255.0")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to bring up interface: %v", err)
	}
	return nil
}

func (tun *TUNInterface) Read(buffer []byte) (int, error) {
	return tun.iface.Read(buffer)
}

func (tun *TUNInterface) Write(buffer []byte) (int, error) {
	return tun.iface.Write(buffer)
}
func (tun *TUNInterface) Name() string {
	return tun.name
}

func (tun *TUNInterface) Close() error {
	if runtime.GOOS != "linux" && tun.isServer {
		cmd := exec.Command("iptables", "-t", "nat", "-D", "POSTROUTING",
			"-s", tun.subnet, "-o", getDefaultInterface(), "-j", "MASQUERADE")
		_ = cmd.Run()
	}
	return tun.iface.Close()
}

func getDefaultInterface() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "eth0"
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagLoopback == 0 {
			addrs, _ := iface.Addrs()
			for _, addr := range addrs {
				if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
					if ipnet.IP.To4() != nil {
						return iface.Name
					}
				}
			}
		}
	}
	return "eth0"
}
