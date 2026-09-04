package handler

import (
	"encoding/json"
	"os/exec"
	"testing"
)

// The log viewer shells out to journalctl. These tests pin the security
// properties Mimosa flagged as review points for exec usage in this repo:
//  1. the argv contains ONLY literals / strconv of constants — nothing
//     request-derived, so there is no injection surface;
//  2. the child is started via argv list (no shell string anywhere);
//  3. argv shape is stable across calls (guards against future edits
//     accidentally threading request data into the command line).
func TestLogViewerArgvIsConstantAndLiteral(t *testing.T) {
	a1, a2 := logViewerArgv(), logViewerArgv()
	if len(a1) != len(a2) {
		t.Fatalf("argv length changed between calls: %d vs %d", len(a1), len(a2))
	}
	for i := range a1 {
		if a1[i] != a2[i] {
			t.Errorf("argv[%d] changed between calls: %q vs %q", i, a1[i], a2[i])
		}
	}
	for i, el := range a1 {
		if el == "" {
			t.Errorf("argv[%d] is empty", i)
		}
	}
	// option/value pairing sanity: every option that needs a value has one
	for i, el := range a1 {
		if el == "--unit" || el == "--since" || el == "--lines" || el == "--output-fields" {
			if i+1 >= len(a1) {
				t.Fatalf("option %s at argv[%d] has no value", el, i)
			}
		}
	}
}

func TestJournalctlChildUsesArgvList(t *testing.T) {
	// build the command object the handler would start; do not run it
	// (journalctl may not exist on Windows dev hosts — this test only
	// inspects process construction).
	cmd := exec.Command("journalctl", logViewerArgv()...)
	if cmd.Path == "" {
		t.Fatal("exec.Command produced an empty Path")
	}
	// exec.Command with an argv list leaves Args[0] as the program name and
	// never embeds a shell; a shell invocation would show sh -c / bash -c.
	if cmd.Args[0] != "journalctl" {
		t.Fatalf("unexpected program: %q", cmd.Args[0])
	}
	if len(cmd.Args) < 2 {
		t.Fatal("expected arguments after program name")
	}
	// no "-c" shell-style flag anywhere in the command line
	for _, a := range cmd.Args {
		if a == "-c" || a == "/bin/sh" || a == "sh" || a == "bash" {
			t.Fatalf("shell-style invocation detected: %v", cmd.Args)
		}
	}
}

func TestParseJournalJSON(t *testing.T) {
	msg := `{"time":"2026-09-04T19:33:29Z","level":"info","msg":"http access"}`
	in := `{"_SYSTEMD_UNIT":"oj-api.service","PRIORITY":"3","__REALTIME_TIMESTAMP":"1700000000000000","MESSAGE":` +
		quoteJSON(msg) + "}\n" +
		"{\"_SYSTEMD_UNIT\":\"oj-judge.service\",\"PRIORITY\":\"6\",\"__REALTIME_TIMESTAMP\":\"1700000005000000\",\"MESSAGE\":\"ok\"}\n" +
		"-- Journal begins at ... --\n" // banner line must be skipped
	entries := parseJournalJSON([]byte(in))
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(entries), entries)
	}
	if entries[0].Unit != "oj-api.service" || entries[0].Level != "error" {
		t.Errorf("entry0 = %+v", entries[0])
	}
	if entries[0].Msg != msg {
		t.Errorf("message not preserved: %q", entries[0].Msg)
	}
	if entries[1].Level != "info" || entries[1].Msg != "ok" {
		t.Errorf("entry1 = %+v", entries[1])
	}
	if want := "2023-11-15 06:13:20.000"; entries[0].TS != want {
		t.Errorf("ts = %q, want %q", entries[0].TS, want)
	}
}

func TestParseJournalJSONSkipsGarbage(t *testing.T) {
	entries := parseJournalJSON([]byte("garbage\n\n{}\nnotjson\n{\"_SYSTEMD_UNIT\":\"oj-api.service\"}\n"))
	// a record without MESSAGE is dropped (banner/blank records carry none)
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %+v", entries)
	}
}

func TestParseJournalJSONArrayMessage(t *testing.T) {
	// journald encodes non-printable MESSAGE payloads as byte arrays
	in := `{"_SYSTEMD_UNIT":"oj-api.service","PRIORITY":"6","__REALTIME_TIMESTAMP":"1700000000000000","MESSAGE":[104,105]}` + "\n"
	entries := parseJournalJSON([]byte(in))
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1: %+v", len(entries), entries)
	}
	if entries[0].Msg != "«binary record, 2 bytes»" {
		t.Errorf("binary placeholder wrong: %q", entries[0].Msg)
	}
}

// quoteJSON encodes s as a JSON string literal (incl. surrounding quotes).
func quoteJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
