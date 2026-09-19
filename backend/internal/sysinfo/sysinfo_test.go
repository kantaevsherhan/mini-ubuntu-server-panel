package sysinfo

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestListeningPortsParsesProcTables(t *testing.T) {
	root := t.TempDir()
	header := "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n"
	writeFile(t, root, "proc/net/tcp", header+
		"   0: 00000000:0016 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 1111 1 0\n"+
		"   1: 0100007F:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 2222 1 0\n"+
		"   2: 0100007F:1F91 0100007F:C000 01 00000000:00000000 00:00000000 00000000  1000        0 3333 1 0\n")
	writeFile(t, root, "proc/net/tcp6", header+
		"   0: 00000000000000000000000000000000:0050 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 4444 1 0\n")
	writeFile(t, root, "proc/net/udp", header+
		"   0: 00000000:0035 00000000:0000 07 00000000:00000000 00:00000000 00000000     0        0 5555 2 0\n")

	ports := (&Collector{Root: root}).ListeningPorts()
	got := []string{}
	for _, port := range ports {
		got = append(got, port.Protocol+" "+port.Address+" "+strconv.Itoa(port.Port))
	}
	want := "tcp 0.0.0.0 22,udp 0.0.0.0 53,tcp :: 80,tcp 127.0.0.1 8080"
	if strings.Join(got, ",") != want {
		t.Fatalf("unexpected ports: %v", got)
	}
	if ports[3].Public || !ports[0].Public {
		t.Fatalf("unexpected public flags: %#v", ports)
	}
}

func TestInfoReadsProcFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "etc/os-release", "NAME=Ubuntu\nPRETTY_NAME=\"Ubuntu 24.04 LTS\"\n")
	writeFile(t, root, "proc/sys/kernel/osrelease", "6.8.0-45-generic\n")
	writeFile(t, root, "proc/uptime", "3600.50 7000.00\n")
	writeFile(t, root, "proc/loadavg", "0.10 0.20 0.30 1/200 999\n")
	writeFile(t, root, "proc/meminfo", "MemTotal:       2048 kB\nMemAvailable:   1024 kB\nSwapTotal: 0 kB\n")
	writeFile(t, root, "proc/mounts", "/dev/sda1 / ext4 rw 0 0\nproc /proc proc rw 0 0\n/dev/sda1 /snap ext4 rw 0 0\n")

	info := (&Collector{Root: root}).Info()
	if info.OS != "Ubuntu 24.04 LTS" || info.Kernel != "6.8.0-45-generic" || info.UptimeSeconds != 3600.5 || info.Load[2] != 0.3 {
		t.Fatalf("unexpected info: %#v", info)
	}
	if info.Memory.TotalBytes != 2048*1024 || info.Memory.AvailableBytes != 1024*1024 {
		t.Fatalf("unexpected memory: %#v", info.Memory)
	}
	if len(info.Disks) != 1 || info.Disks[0].Mountpoint != "/" || info.Disks[0].FSType != "ext4" {
		t.Fatalf("unexpected disks: %#v", info.Disks)
	}
}
