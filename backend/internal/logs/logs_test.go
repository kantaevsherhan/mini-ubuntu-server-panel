package logs

import (
	"strings"
	"testing"
)

func TestValidateQueryUsesAllowlists(t *testing.T) {
	valid := Normalize(Query{Unit: "ssh.service", Priority: "warning", Range: "week", Limit: 500})
	if err := Validate(valid); err != nil {
		t.Fatal(err)
	}
	unsafe := []Query{
		{Unit: "ssh.service;reboot", Priority: "info", Range: "day", Limit: 10},
		{Priority: "0..7", Range: "day", Limit: 10},
		{Priority: "info", Range: "all", Limit: 10},
		{Priority: "info", Range: "day", Limit: 2001},
	}
	for _, query := range unsafe {
		if err := Validate(query); err == nil {
			t.Fatalf("unsafe query accepted: %#v", query)
		}
	}
}

func TestParseEntriesTruncatesAndSkipsMalformedRows(t *testing.T) {
	longMessage := strings.Repeat("x", maxMessageBytes+10)
	data := []byte("not-json\n" + `{"__REALTIME_TIMESTAMP":"1720000000000000","_SYSTEMD_UNIT":"ssh.service","PRIORITY":"3","MESSAGE":"` + longMessage + `","SYSLOG_IDENTIFIER":"sshd","_PID":"42"}` + "\n")
	entries := parseEntries(data, 10)
	if len(entries) != 1 || entries[0].Unit != "ssh.service" || entries[0].Identifier != "sshd" || len(entries[0].Message) != maxMessageBytes {
		t.Fatalf("unexpected entries: %#v", entries)
	}
}
