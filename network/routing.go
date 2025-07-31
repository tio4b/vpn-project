package network

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"net"
	"os/exec"
	"runtime"
	"strings"
)

type RouteManager struct {
	tunName     string
	serverIP    string
	originalGW  string
	originalDNS []string
	vpnDNS      []string
}

func NewRouteManager(tunName, serverIP string, vpnDNS []string) *RouteManager {
	return &RouteManager{
		tunName:  tunName,
		serverIP: serverIP,
		vpnDNS:   vpnDNS,
	}
}

func (r *RouteManager) SetupClientRoutes() error {
	gw, err := r.getDefaultGateway()
	if err != nil {
		return fmt.Errorf("get default gateway: %v", err)
	}
	r.originalGW = gw
	r.originalDNS = r.getCurrentDNS()
	switch runtime.GOOS {
	case "linux":
		return r.SetupLinuxRoutes()
	case "darwin":
		return r.SetupDarwinRoutes()
	case "windows":
		return r.SetupWindowsRoutes()
	default:
		return fmt.Errorf("unsupported platform %s", runtime.GOOS)
	}
}

func (r *RouteManager) SetupLinuxRoutes() error {
	serverHost, _, _ := net.SplitHostPort(r.serverIP)
	cmd := exec.Command("ip", "route", "add", serverHost, "via", r.originalGW)
	if err := cmd.Run(); err != nil {
		logrus.Warnf("Failed to setup routes: %v", err)
	}
	cmd = exec.Command("ip", "route", "del", "default")
	_ = cmd.Run()
	cmd = exec.Command("ip", "route", "add", "0.0.0.0/1", "dev", r.tunName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add route 0.0.0.0/1: %v", err)
	}
	cmd = exec.Command("ip", "route", "add", "128.0.0.0/1", "dev", r.tunName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add route 128.0.0.0/1: %v", err)
	}
	if err := r.setupDNS(); err != nil {
		logrus.Warnf("Failed to setup DNS: %v", err)
	}
	return nil
}

func (r *RouteManager) setupDNS() error {
	if len(r.vpnDNS) == 0 {
		return nil
	}
	switch runtime.GOOS {
	case "linux":
		_ = exec.Command("cp", "/etc/resolv.conf", "/etc/resolv.conf.vpnbackup").Run()
		content := ""
		for _, dns := range r.vpnDNS {
			content += fmt.Sprintf("nameserver %s\n", dns)
		}
		cmd := exec.Command("sh", "-c", fmt.Sprintf("echo '%s' > /etc/resolv.conf", content))
		return cmd.Run()
	case "darwin":
		services, _ := exec.Command("networksetup", "-listallnetworkservices").Output()
		serviceList := strings.Split(string(services), "\n")
		for _, service := range serviceList {
			service = strings.TrimSpace(service)
			if service != "" && !strings.Contains(service, "*") {
				args := []string{"-setdnsservers", service}
				args = append(args, r.vpnDNS...)
				exec.Command("networksetup", args...).Run()
			}
		}
	}
	return nil
}

func (r *RouteManager) SetupWindowsRoutes() error {
	serverHost, _, _ := net.SplitHostPort(r.serverIP)
	cmd := exec.Command("route", "add", serverHost, "mask", "255.255.255.255", r.originalGW)
	if err := cmd.Run(); err != nil {
		logrus.Warnf("Failed to add server route: %v", err)
	}
	tunIP := r.getTUNIP()
	cmd = exec.Command("route", "add", "0.0.0.0", "mask", "128.0.0.0", tunIP)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add route 0.0.0.0/1: %v", err)
	}
	cmd = exec.Command("route", "add", "128.0.0.0", "mask", "128.0.0.0", tunIP)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add route 128.0.0.0/1: %v", err)
	}
	return nil
}

func (r *RouteManager) getTUNIP() string {
	iface, err := net.InterfaceByName(r.tunName)
	if err != nil {
		return "10.0.0.2"
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return "10.0.0.2"
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil {
			return ipnet.IP.String()
		}
	}
	return "10.0.0.2"
}

func (r *RouteManager) SetupDarwinRoutes() error {
	serverHost, _, _ := net.SplitHostPort(r.serverIP)
	cmd := exec.Command("route", "add", "-host", serverHost, r.originalGW)
	if err := cmd.Run(); err != nil {
		logrus.Warnf("Failed to add server route: %v", err)
	}
	cmd = exec.Command("route", "add", "-net", "0.0.0.0/1", "-interface", r.tunName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add route 0.0.0.0/1: %v", err)
	}
	cmd = exec.Command("route", "add", "-net", "128.0.0.0/1", "-interface", r.tunName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add route 128.0.0.0/1: %v", err)
	}
	if err := r.setupDNS(); err != nil {
		logrus.Warnf("Failed to setup DNS: %v", err)
	}

	return nil
}

func (r *RouteManager) getDefaultGateway() (string, error) {
	switch runtime.GOOS {
	case "linux":
		cmd := exec.Command("ip", "route", "show", "default")
		output, err := cmd.Output()
		if err != nil {
			return "", err
		}
		fields := strings.Fields(string(output))
		for i, field := range fields {
			if field == "via" && i+1 < len(fields) {
				return fields[i+1], nil
			}
		}
	case "darwin":
		cmd := exec.Command("route", "-n", "get", "default")
		output, err := cmd.Output()
		if err != nil {
			return "", err
		}
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, "gateway:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					return fields[1], nil
				}
			}
		}
	case "windows":
		cmd := exec.Command("route", "print", "0.0.0.0")
		output, err := cmd.Output()
		if err != nil {
			return "", err
		}
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, "0.0.0.0") && strings.Contains(line, "0.0.0.0") {
				fields := strings.Fields(line)
				if len(fields) >= 3 {
					return fields[2], nil
				}
			}
		}
	}
	return "", fmt.Errorf("failed to find default gateway")
}

func (r *RouteManager) getCurrentDNS() []string {
	var dns []string
	switch runtime.GOOS {
	case "linux":
		data, err := exec.Command("cat", "/etc/resolv.conf").Output()
		if err != nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if strings.Contains(line, "nameserver") {
					fields := strings.Fields(line)
					if len(fields) >= 2 {
						dns = append(dns, fields[1])
					}
				}
			}
		}
	case "darwin":
		out, err := exec.Command("scutil", "--dns").Output()
		if err != nil {
			lines := strings.Split(string(out), "\n")
			for _, line := range lines {
				if strings.Contains(line, "nameserver") {
					fields := strings.Fields(line)
					if len(fields) >= 3 {
						dns = append(dns, fields[2])
					}
				}
			}
		}
	}
	return dns
}

func (r *RouteManager) RestoreRoutes() error {
	switch runtime.GOOS {
	case "linux":
		return r.restoreLinuxRoutes()
	case "darwin":
		return r.restoreDarwinRoutes()
	case "windows":
		return r.restoreWindowsRoutes()
	default:
		return nil
	}
}

func (r *RouteManager) restoreLinuxRoutes() error {
	exec.Command("ip", "route", "del", "0.0.0.0/1").Run()
	exec.Command("ip", "route", "del", "128.0.0.0/1").Run()
	serverHost, _, _ := net.SplitHostPort(r.serverIP)
	exec.Command("ip", "route", "del", serverHost).Run()
	if r.originalGW != "" {
		cmd := exec.Command("ip", "route", "add", "default", "via", r.originalGW)
		cmd.Run()
	}
	r.restoreDNS()

	return nil
}

func (r *RouteManager) restoreDarwinRoutes() error {
	exec.Command("route", "delete", "-net", "0.0.0.0/1").Run()
	exec.Command("route", "delete", "-net", "128.0.0.0/1").Run()
	serverHost, _, _ := net.SplitHostPort(r.serverIP)
	exec.Command("route", "delete", "-host", serverHost).Run()
	r.restoreDNS()
	return nil
}
func (r *RouteManager) restoreWindowsRoutes() error {
	exec.Command("route", "delete", "0.0.0.0").Run()
	exec.Command("route", "delete", "128.0.0.0").Run()
	serverHost, _, _ := net.SplitHostPort(r.serverIP)
	exec.Command("route", "delete", serverHost).Run()
	return nil
}

func (r *RouteManager) restoreDNS() {
	switch runtime.GOOS {
	case "linux":
		exec.Command("mv", "/etc/resolv.conf.vpnbackup", "/etc/resolv.conf").Run()
	case "darwin":
		services, _ := exec.Command("networksetup", "-listallnetworkservices").Output()
		serviceList := strings.Split(string(services), "\n")
		for _, service := range serviceList {
			service = strings.TrimSpace(service)
			if service != "" && !strings.Contains(service, "*") {
				_ = exec.Command("networksetup", "-setdnsservers", service, "Empty").Run()
			}
		}
	}
}
