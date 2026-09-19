// Package sysinfo reads host facts (OS, uptime, disks, interfaces, listening sockets) from /proc and /sys without privileges.
package sysinfo

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

type Collector struct {
	// Root is prepended to /proc, /etc and /sys paths; "/" in production, a temp dir in tests.
	Root string
}

type Info struct {
	Hostname      string      `json:"hostname"`
	OS            string      `json:"os"`
	Kernel        string      `json:"kernel"`
	Arch          string      `json:"arch"`
	CPUCount      int         `json:"cpu_count"`
	CPUModel      string      `json:"cpu_model"`
	UptimeSeconds float64     `json:"uptime_seconds"`
	Load          [3]float64  `json:"load"`
	Memory        Memory      `json:"memory"`
	Disks         []Disk      `json:"disks"`
	Interfaces    []Interface `json:"interfaces"`
	Timezone      string      `json:"timezone"`
	RebootNeeded  bool        `json:"reboot_required"`
	Updates       string      `json:"updates_summary"`
}

type Memory struct {
	TotalBytes     uint64 `json:"total_bytes"`
	AvailableBytes uint64 `json:"available_bytes"`
	SwapTotalBytes uint64 `json:"swap_total_bytes"`
	SwapFreeBytes  uint64 `json:"swap_free_bytes"`
}

type Disk struct {
	Device     string `json:"device"`
	Mountpoint string `json:"mountpoint"`
	FSType     string `json:"fs_type"`
	TotalBytes uint64 `json:"total_bytes"`
	UsedBytes  uint64 `json:"used_bytes"`
	FreeBytes  uint64 `json:"free_bytes"`
}

type Interface struct {
	Name      string   `json:"name"`
	MAC       string   `json:"mac"`
	Up        bool     `json:"up"`
	MTU       int      `json:"mtu"`
	Addresses []string `json:"addresses"`
	RxBytes   uint64   `json:"rx_bytes"`
	TxBytes   uint64   `json:"tx_bytes"`
}

type ListeningPort struct {
	Protocol string `json:"protocol"`
	Address  string `json:"address"`
	Port     int    `json:"port"`
	PID      int    `json:"pid,omitempty"`
	Process  string `json:"process,omitempty"`
	Public   bool   `json:"public"`
}

var diskFSTypes = map[string]bool{"ext2": true, "ext3": true, "ext4": true, "xfs": true, "btrfs": true, "zfs": true, "f2fs": true, "vfat": true, "exfat": true, "ntfs": true, "ntfs3": true, "jfs": true, "reiserfs": true}

func New() *Collector { return &Collector{Root: "/"} }

func (c *Collector) path(parts ...string) string {
	return filepath.Join(append([]string{c.Root}, parts...)...)
}

func (c *Collector) Info() Info {
	hostname, _ := os.Hostname()
	info := Info{Hostname: hostname, Arch: runtime.GOARCH, CPUCount: runtime.NumCPU()}
	info.OS = c.osName()
	info.Kernel = strings.TrimSpace(c.readSmall("proc", "sys", "kernel", "osrelease"))
	if fields := strings.Fields(c.readSmall("proc", "uptime")); len(fields) > 0 {
		info.UptimeSeconds, _ = strconv.ParseFloat(fields[0], 64)
	}
	if fields := strings.Fields(c.readSmall("proc", "loadavg")); len(fields) >= 3 {
		for index := range 3 {
			info.Load[index], _ = strconv.ParseFloat(fields[index], 64)
		}
	}
	info.CPUModel = c.cpuModel()
	info.Memory = c.memory()
	info.Disks = c.disks()
	info.Interfaces = c.interfaces()
	info.Timezone = c.timezone()
	_, err := os.Stat(c.path("var", "run", "reboot-required"))
	info.RebootNeeded = err == nil
	// Written by update-notifier-common on Ubuntu; reading it avoids running apt on every request.
	info.Updates = strings.TrimSpace(c.readSmall("var", "lib", "update-notifier", "updates-available"))
	return info
}

func (c *Collector) readSmall(parts ...string) string {
	file, err := os.Open(c.path(parts...))
	if err != nil {
		return ""
	}
	defer file.Close()
	data, _ := io.ReadAll(io.LimitReader(file, 64*1024))
	return string(data)
}

func (c *Collector) osName() string {
	for line := range strings.SplitSeq(c.readSmall("etc", "os-release"), "\n") {
		if value, ok := strings.CutPrefix(line, "PRETTY_NAME="); ok {
			return strings.Trim(value, `"'`)
		}
	}
	return runtime.GOOS
}

func (c *Collector) cpuModel() string {
	for line := range strings.SplitSeq(c.readSmall("proc", "cpuinfo"), "\n") {
		key, value, found := strings.Cut(line, ":")
		if found && (strings.TrimSpace(key) == "model name" || strings.TrimSpace(key) == "Model") {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (c *Collector) memory() Memory {
	values := map[string]uint64{}
	for line := range strings.SplitSeq(c.readSmall("proc", "meminfo"), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			value, _ := strconv.ParseUint(fields[1], 10, 64)
			values[strings.TrimSuffix(fields[0], ":")] = value * 1024
		}
	}
	return Memory{TotalBytes: values["MemTotal"], AvailableBytes: values["MemAvailable"], SwapTotalBytes: values["SwapTotal"], SwapFreeBytes: values["SwapFree"]}
}

func (c *Collector) disks() []Disk {
	disks := []Disk{}
	seen := map[string]bool{}
	for line := range strings.SplitSeq(c.readSmall("proc", "mounts"), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 || !diskFSTypes[fields[2]] || seen[fields[0]] {
			continue
		}
		mountpoint := unescapeMount(fields[1])
		// Bind-mounted files (e.g. /etc/resolv.conf in containers) share the device but are not filesystems.
		if stat, err := os.Stat(c.path(mountpoint)); err == nil && !stat.IsDir() {
			continue
		}
		seen[fields[0]] = true
		disk := Disk{Device: fields[0], Mountpoint: mountpoint, FSType: fields[2]}
		if total, free, available, err := statfs(c.path(mountpoint)); err == nil {
			disk.TotalBytes, disk.FreeBytes = total, available
			disk.UsedBytes = total - free
		}
		disks = append(disks, disk)
	}
	sort.Slice(disks, func(i, j int) bool { return disks[i].Mountpoint < disks[j].Mountpoint })
	return disks
}

func unescapeMount(value string) string {
	replacer := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	return replacer.Replace(value)
}

func (c *Collector) interfaceCounters() map[string][2]uint64 {
	counters := map[string][2]uint64{}
	for line := range strings.SplitSeq(c.readSmall("proc", "net", "dev"), "\n") {
		name, rest, found := strings.Cut(line, ":")
		fields := strings.Fields(rest)
		if !found || len(fields) < 9 {
			continue
		}
		rx, _ := strconv.ParseUint(fields[0], 10, 64)
		tx, _ := strconv.ParseUint(fields[8], 10, 64)
		counters[strings.TrimSpace(name)] = [2]uint64{rx, tx}
	}
	return counters
}

func (c *Collector) interfaces() []Interface {
	counters := c.interfaceCounters()
	items := []Interface{}
	list, err := net.Interfaces()
	if err != nil {
		return items
	}
	for _, iface := range list {
		item := Interface{Name: iface.Name, MAC: iface.HardwareAddr.String(), Up: iface.Flags&net.FlagUp != 0, MTU: iface.MTU, Addresses: []string{}}
		if addrs, err := iface.Addrs(); err == nil {
			for _, addr := range addrs {
				item.Addresses = append(item.Addresses, addr.String())
			}
		}
		item.RxBytes, item.TxBytes = counters[iface.Name][0], counters[iface.Name][1]
		items = append(items, item)
	}
	return items
}

// ListeningPorts parses /proc/net/{tcp,tcp6,udp,udp6}. Owning processes are resolved only where /proc/<pid>/fd is readable.
func (c *Collector) ListeningPorts() []ListeningPort {
	owners := c.socketOwners()
	ports := []ListeningPort{}
	seen := map[string]bool{}
	for _, source := range []struct{ file, protocol, state string }{{"tcp", "tcp", "0A"}, {"tcp6", "tcp", "0A"}, {"udp", "udp", "07"}, {"udp6", "udp", "07"}} {
		file, err := os.Open(c.path("proc", "net", source.file))
		if err != nil {
			continue
		}
		for _, port := range parseSocketTable(file, source.protocol, source.state) {
			if owner, ok := owners[port.inode]; ok {
				port.PID, port.Process = owner.pid, owner.name
			}
			key := fmt.Sprintf("%s|%s|%d", port.Protocol, port.Address, port.Port)
			if !seen[key] {
				seen[key] = true
				ports = append(ports, port.ListeningPort)
			}
		}
		file.Close()
	}
	sort.Slice(ports, func(i, j int) bool {
		if ports[i].Port != ports[j].Port {
			return ports[i].Port < ports[j].Port
		}
		return ports[i].Protocol+ports[i].Address < ports[j].Protocol+ports[j].Address
	})
	return ports
}

type socketEntry struct {
	ListeningPort
	inode string
}

func parseSocketTable(reader io.Reader, protocol, listenState string) []socketEntry {
	entries := []socketEntry{}
	scanner := bufio.NewScanner(io.LimitReader(reader, 16<<20))
	scanner.Scan() // header
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 10 || fields[3] != listenState {
			continue
		}
		hostHex, portHex, found := strings.Cut(fields[1], ":")
		if !found {
			continue
		}
		port, err := strconv.ParseUint(portHex, 16, 16)
		if err != nil {
			continue
		}
		ip := decodeProcIP(hostHex)
		if ip == nil {
			continue
		}
		entries = append(entries, socketEntry{ListeningPort: ListeningPort{Protocol: protocol, Address: ip.String(), Port: int(port), Public: !ip.IsLoopback()}, inode: fields[9]})
	}
	return entries
}

// decodeProcIP converts the little-endian-per-word hex encoding used by /proc/net/*.
func decodeProcIP(value string) net.IP {
	raw, err := hex.DecodeString(value)
	if err != nil || (len(raw) != 4 && len(raw) != 16) {
		return nil
	}
	ip := make(net.IP, len(raw))
	for word := 0; word < len(raw); word += 4 {
		for index := range 4 {
			ip[word+index] = raw[word+3-index]
		}
	}
	if v4 := ip.To4(); v4 != nil && len(raw) == 16 {
		return v4
	}
	return ip
}

type socketOwner struct {
	pid  int
	name string
}

func (c *Collector) socketOwners() map[string]socketOwner {
	owners := map[string]socketOwner{}
	entries, err := os.ReadDir(c.path("proc"))
	if err != nil {
		return owners
	}
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		fdDir := c.path("proc", entry.Name(), "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}
		name := ""
		for _, fd := range fds {
			target, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil || !strings.HasPrefix(target, "socket:[") {
				continue
			}
			if name == "" {
				name = strings.TrimSpace(c.readSmall("proc", entry.Name(), "comm"))
			}
			owners[strings.TrimSuffix(strings.TrimPrefix(target, "socket:["), "]")] = socketOwner{pid: pid, name: name}
		}
	}
	return owners
}

func (c *Collector) timezone() string {
	if value := strings.TrimSpace(c.readSmall("etc", "timezone")); value != "" {
		return value
	}
	if target, err := os.Readlink(c.path("etc", "localtime")); err == nil {
		if _, zone, found := strings.Cut(target, "zoneinfo/"); found {
			return zone
		}
	}
	return "UTC"
}
