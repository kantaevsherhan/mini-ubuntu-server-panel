package services

import "testing"

func TestValidateActionAllowlistAndProtectPanel(t *testing.T) {
	if err := ValidateAction("ssh.service", "restart"); err != nil {
		t.Fatal(err)
	}
	unsafe := []actionRequest{
		{Unit: "ssh.service;reboot", Action: "start"},
		{Unit: "../ssh.service", Action: "start"},
		{Unit: "ssh.service", Action: "mask"},
		{Unit: "mini-ubuntu-server.service", Action: "stop"},
	}
	for _, request := range unsafe {
		if err := ValidateAction(request.Unit, request.Action); err == nil {
			t.Fatalf("unsafe request accepted: %#v", request)
		}
	}
}

func TestParseUnitsJoinsDescriptionAndEnabledState(t *testing.T) {
	items := parseUnits("ssh.service loaded active running OpenBSD Secure Shell server\n", "cron.service disabled enabled\nssh.service enabled enabled\n")
	if len(items) != 2 || items[1].Name != "ssh.service" || items[1].Description != "OpenBSD Secure Shell server" || items[1].Enabled != "enabled" || items[0].Name != "cron.service" || items[0].ActiveState != "inactive" {
		t.Fatalf("unexpected services: %#v", items)
	}
}
