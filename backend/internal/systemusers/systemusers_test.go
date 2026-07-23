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
