package firewall

import "testing"

func TestValidateRuleAllowlistAndSSHProtection(t *testing.T) {
	if err := ValidateRule(AddRequest{Action: "allow", Port: 443, Protocol: "tcp", Source: "10.0.0.0/8"}); err != nil {
		t.Fatal(err)
	}
	unsafe := []AddRequest{
		{Action: "deny", Port: 22, Protocol: "tcp", Source: "any"},
		{Action: "reject", Port: 80, Protocol: "tcp", Source: "any"},
		{Action: "allow", Port: 0, Protocol: "tcp", Source: "any"},
		{Action: "allow", Port: 53, Protocol: "all", Source: "any"},
		{Action: "allow", Port: 80, Protocol: "tcp", Source: "10.0.0.1;reboot"},
	}
	for _, rule := range unsafe {
		if err := ValidateRule(rule); err == nil {
			t.Fatalf("unsafe rule accepted: %#v", rule)
		}
	}
}

func TestParseStatus(t *testing.T) {
	status := parseStatus("Status: active\n\n[ 1] 22/tcp                     ALLOW IN    Anywhere\n[ 2] 443/tcp                    DENY IN     10.0.0.0/8\n")
	if !status.Active || len(status.Rules) != 2 || status.Rules[0].Number != 1 || status.Rules[1].Action != "deny" || status.Rules[1].From != "10.0.0.0/8" {
		t.Fatalf("unexpected status: %#v", status)
	}
}
