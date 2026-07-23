package processes

import "testing"

func TestParseStatHandlesSpacesAndParenthesesInName(t *testing.T) {
	name, fields, err := parseStat("123 (worker (one) task) S 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20 21 22")
	if err != nil || name != "worker (one) task" || len(fields) != 23 || fields[0] != "S" {
		t.Fatalf("unexpected parse result: name=%q fields=%v err=%v", name, fields, err)
	}
}

func TestResolveSignalUsesAllowlist(t *testing.T) {
	if _, err := resolveSignal(signalRequest{PID: 42, Signal: "TERM"}); err != nil {
		t.Fatal(err)
	}
	for _, request := range []signalRequest{{PID: 1, Signal: "TERM"}, {PID: 42, Signal: "STOP"}, {PID: -1, Signal: "KILL"}} {
		if _, err := resolveSignal(request); err == nil {
			t.Fatalf("unsafe request accepted: %#v", request)
		}
	}
}
