package systemusers

import (
	"os/user"
	"testing"
)

func TestValidateCreateAcceptsSafeRequest(t *testing.T) {
	request := CreateRequest{
		Username: "panel_test", HomeDirectory: "/home/panel_test", Shell: "/bin/bash",
		CreateHome: true, AllowSSH: true,
		SSHPublicKey: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIG7eN6WvMYdl5iIqnUsSTQmIa5Aq8n9cIYQzS0Vq3R7 test",
	}
	if err := validateCreate(request); err != nil {
		t.Fatalf("safe request rejected: %v", err)
	}
}

func TestValidateCreateRejectsTraversalAndShell(t *testing.T) {
	cases := []CreateRequest{
		{Username: "panel_test", HomeDirectory: "/home/panel_test/..", Shell: "/bin/bash"},
		{Username: "panel_test", HomeDirectory: "/etc", Shell: "/bin/bash"},
		{Username: "panel_test", HomeDirectory: "/home/panel_test", Shell: "/bin/bash;id"},
		{Username: "root", HomeDirectory: "/home/root", Shell: "/bin/bash"},
		{Username: "panel_test", HomeDirectory: "/home/panel_test", Shell: "/bin/bash", AllowSSH: true},
	}
	for index, request := range cases {
		if err := validateCreate(request); err == nil {
			t.Fatalf("unsafe request %d accepted", index)
		}
	}
}

func TestValidateCreateRejectsUnknownGroup(t *testing.T) {
	name := "group_that_must_not_exist_12345"
	if _, err := user.LookupGroup(name); err == nil {
		t.Skip("test group unexpectedly exists")
	}
	request := CreateRequest{Username: "panel_test", HomeDirectory: "/home/panel_test", Shell: "/bin/bash", Groups: []string{name}}
	if err := validateCreate(request); err == nil {
		t.Fatal("unknown group accepted")
	}
}

func TestValidSSHKey(t *testing.T) {
	if !validSSHKey("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIG7eN6WvMYdl5iIqnUsSTQmIa5Aq8n9cIYQzS0Vq3R7") {
		t.Fatal("valid key rejected")
	}
	if validSSHKey("command=whoami ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIG7eN6WvMYdl5iIqnUsSTQmIa5Aq8n9cIYQzS0Vq3R7") {
		t.Fatal("authorized_keys options must not be accepted")
	}
}

func TestParseWhoFiltersUserAndNormalizesTime(t *testing.T) {
	sessions := parseWho("alice", "alice pts/0 2026-07-23 18:30 (192.0.2.10)\nbob pts/1 2026-07-23 18:31 (192.0.2.11)\n")
	if len(sessions) != 1 || sessions[0].Terminal != "pts/0" || sessions[0].RemoteIP != "192.0.2.10" {
		t.Fatalf("unexpected sessions: %#v", sessions)
	}
}

func TestParseLastLogin(t *testing.T) {
	value := parseLastLogin("alice pts/0 192.0.2.10 Thu Jul 23 18:30:01 2026 - Thu Jul 23 18:40:00 2026 (00:09)\n")
	if value == nil || value.Year() != 2026 || value.Month() != 7 || value.Day() != 23 {
		t.Fatalf("unexpected last login: %v", value)
	}
	if value := parseLastLogin("wtmp begins Thu Jul 23 00:00:00 2026"); value != nil {
		t.Fatalf("wtmp header must not be treated as a login: %v", value)
	}
}
