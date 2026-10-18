package aptupdates

import (
	"os"
	"path/filepath"
	"testing"
)

const simulation = `NOTE: This is only a simulation!
      apt-get needs root privileges for real execution.
Reading package lists...
The following packages will be upgraded:
  libssl3t64 openssl tzdata
Inst tzdata [2024a-3ubuntu1] (2024b-0ubuntu0.24.04 Ubuntu:24.04/noble-updates [all])
Inst openssl [3.0.13-0ubuntu3.4] (3.0.13-0ubuntu3.5 Ubuntu:24.04/noble-updates, Ubuntu:24.04/noble-security [amd64])
Inst linux-image-6.8.0-50-generic (6.8.0-50.51 Ubuntu:24.04/noble-security [amd64])
Conf tzdata (2024b-0ubuntu0.24.04 Ubuntu:24.04/noble-updates [all])
`

func TestParseSimulation(t *testing.T) {
	packages := ParseSimulation(simulation)
	if len(packages) != 3 {
		t.Fatalf("expected 3 packages, got %#v", packages)
	}
	// Security updates sort first, then by name.
	if packages[0].Name != "linux-image-6.8.0-50-generic" || !packages[0].Security || !packages[0].New || packages[0].Current != "" {
		t.Fatalf("unexpected new security package: %#v", packages[0])
	}
	if packages[1].Name != "openssl" || packages[1].Current != "3.0.13-0ubuntu3.4" || packages[1].Candidate != "3.0.13-0ubuntu3.5" || !packages[1].Security {
		t.Fatalf("unexpected openssl entry: %#v", packages[1])
	}
	if packages[2].Name != "tzdata" || packages[2].Security || packages[2].Origin != "Ubuntu:24.04/noble-updates" {
		t.Fatalf("unexpected tzdata entry: %#v", packages[2])
	}
}

func TestParseUnitStates(t *testing.T) {
	cases := map[string]struct{ output, state string }{
		"missing":   {"LoadState=not-found\nActiveState=inactive\n", "idle"},
		"running":   {"LoadState=loaded\nActiveState=active\nSubState=running\nDescription=mini-ubuntu-server apt upgrade\nInvocationID=abc\nExecMainStartTimestamp=@1700000000\n", "running"},
		"succeeded": {"LoadState=loaded\nActiveState=active\nSubState=exited\nExecMainStatus=0\nDescription=mini-ubuntu-server apt refresh\nInvocationID=abc\nExecMainExitTimestamp=Sun 2026-10-18 07:13:40 UTC\n", "succeeded"},
		"failed":    {"LoadState=loaded\nActiveState=failed\nSubState=failed\nExecMainStatus=100\nDescription=mini-ubuntu-server apt upgrade\nInvocationID=abc\n", "failed"},
	}
	for name, tc := range cases {
		job, invocation := ParseUnit(tc.output)
		if job.State != tc.state {
			t.Errorf("%s: state %q, want %q", name, job.State, tc.state)
		}
		if tc.state == "idle" && invocation != "" {
			t.Errorf("%s: idle job must not expose an invocation", name)
		}
	}
	job, _ := ParseUnit(cases["failed"].output)
	if job.ExitCode == nil || *job.ExitCode != 100 || job.Operation != "upgrade" {
		t.Fatalf("failed job details: %#v", job)
	}
	job, _ = ParseUnit(cases["succeeded"].output)
	if job.FinishedAt == nil || job.FinishedAt.Year() != 2026 || job.Operation != "refresh" {
		t.Fatalf("succeeded job details: %#v", job)
	}
	job, _ = ParseUnit(cases["running"].output)
	if job.StartedAt == nil || job.StartedAt.Unix() != 1700000000 || job.FinishedAt != nil {
		t.Fatalf("running job details: %#v", job)
	}
}

func TestValidateRejectsUnsafeRequests(t *testing.T) {
	valid := []privilegedRequest{{Operation: "status"}, {Operation: "refresh"}, {Operation: "upgrade"}, {Operation: "upgrade", Packages: []string{"openssl", "libc6:amd64", "g++-13"}}}
	for _, request := range valid {
		if err := validate(request); err != nil {
			t.Errorf("valid request rejected: %#v", request)
		}
	}
	invalid := []privilegedRequest{
		{Operation: "install"},
		{Operation: "refresh", Packages: []string{"openssl"}},
		{Operation: "upgrade", Packages: []string{"-o=APT::Foo"}},
		{Operation: "upgrade", Packages: []string{"openssl; rm -rf /"}},
		{Operation: "upgrade", Packages: []string{"Upper"}},
		{Operation: "upgrade", Packages: []string{"openssl", "openssl"}},
	}
	for _, request := range invalid {
		if err := validate(request); err == nil {
			t.Errorf("unsafe request accepted: %#v", request)
		}
	}
}

func TestParseAutoUpgradesAndHostFiles(t *testing.T) {
	if !ParseAutoUpgrades("APT::Periodic::Update-Package-Lists \"1\";\nAPT::Periodic::Unattended-Upgrade \"1\";\n") {
		t.Fatal("enabled config not detected")
	}
	if ParseAutoUpgrades("APT::Periodic::Unattended-Upgrade \"0\";") || ParseAutoUpgrades("// APT::Periodic::Unattended-Upgrade \"1\";") {
		t.Fatal("disabled or commented config detected as enabled")
	}
	root := t.TempDir()
	for path, content := range map[string]string{
		"etc/apt/apt.conf.d/20auto-upgrades": "APT::Periodic::Unattended-Upgrade \"1\";\n",
		"usr/bin/unattended-upgrade":         "",
		"var/run/reboot-required":            "",
		"var/run/reboot-required.pkgs":       "linux-base\nlinux-base\nlibc6\n",
	} {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	manager := Manager{Root: root}
	if auto := manager.autoUpgrades(); !auto.Installed || !auto.Enabled {
		t.Fatalf("auto upgrades: %#v", auto)
	}
	if lines := readLines(filepath.Join(root, "var/run/reboot-required.pkgs"), 10); len(lines) != 2 {
		t.Fatalf("reboot packages must be deduplicated: %v", lines)
	}
}
