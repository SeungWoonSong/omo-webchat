package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fixture = `{"type":"system","uuid":"compact","parentUuid":null,"timestamp":"2026-09-15T01:00:00Z","compactMetadata":{"trigger":"auto"}}
{"type":"user","uuid":"summary","parentUuid":"compact","isCompactSummary":true,"message":{"role":"user","content":"Earlier context"}}
{"type":"user","uuid":"u","parentUuid":"summary","sessionId":"old","cwd":"/old/location","message":{"role":"user","content":"Synthetic task"}}
{"type":"assistant","uuid":"a","parentUuid":"u","message":{"role":"assistant","model":"claude-test","content":[{"type":"thinking","thinking":"Reason"},{"type":"text","text":"Answer"},{"type":"tool_use","id":"call-1","name":"Read","input":{"file_path":"fake"}}]}}
{"type":"user","uuid":"r","parentUuid":"a","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"call-1","content":[{"type":"text","text":"Historical output"}]},{"type":"text","text":"Next"}]}}
{"type":"attachment","uuid":"file","parentUuid":"r","attachment":{"type":"file","filename":"synthetic.txt"}}
{"type":"assistant","uuid":"branch","parentUuid":"u","message":{"role":"assistant","content":[{"type":"text","text":"Other branch"}]}}
`

func decoded(t *testing.T, data []byte) []object {
	t.Helper()
	var entries []object
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
		var entry object
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatal(err)
		}
		entries = append(entries, entry)
	}
	return entries
}

func fixtureSource(t *testing.T) source {
	t.Helper()
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(path, []byte(fixture), 0600); err != nil {
		t.Fatal(err)
	}
	records, hash, candidate, err := readRecords(path)
	if err != nil || !candidate {
		t.Fatalf("transcript recognition: candidate=%v err=%v", candidate, err)
	}
	return source{Path: path, CWD: "/project", ID: sessionID(path), Hash: hash, Records: records, Name: "[Claude] Synthetic task"}
}

func TestConversionPreservesBranchesAndToolIdentity(t *testing.T) {
	// Given: two branches and a user record mixing tool output and text.
	s := fixtureSource(t)
	// When: converting historical records.
	data, err := convert(s, "/sessions")
	if err != nil {
		t.Fatal(err)
	}
	// Then: all references resolve; siblings remain siblings, not a flattened log.
	entries := decoded(t, data)
	if entries[0]["version"] != float64(3) || entries[0]["cwd"] != "/project" {
		t.Fatal("invalid OMO header")
	}
	known := map[string]bool{}
	var userID, assistantParent, branchParent, callID, resultID, resultName string
	thinking, toolResults, notes := 0, 0, 0
	for _, e := range entries[1:] {
		if p, ok := e["parentId"].(string); ok && !known[p] {
			t.Fatalf("dangling or forward parent: %s", p)
		}
		known[e["id"].(string)] = true
		if e["type"] == "custom_message" {
			notes++
		}
		msg, ok := e["message"].(map[string]any)
		if !ok {
			continue
		}
		if msg["role"] == "toolResult" {
			toolResults++
			resultID, resultName = msg["toolCallId"].(string), msg["toolName"].(string)
		}
		for _, raw := range msg["content"].([]any) {
			b := raw.(map[string]any)
			switch b["type"] {
			case "thinking":
				thinking++
			case "toolCall":
				callID = b["id"].(string)
				assistantParent = e["parentId"].(string)
			case "text":
				switch b["text"] {
				case "Synthetic task":
					userID = e["id"].(string)
				case "Other branch":
					branchParent = e["parentId"].(string)
				}
			}
		}
	}
	if userID == "" || assistantParent != userID || branchParent != userID {
		t.Fatalf("branch flattened: %q %q %q", userID, assistantParent, branchParent)
	}
	if callID != "call-1" || resultID != callID || resultName != "Read" || toolResults != 1 || thinking != 1 || notes != 3 {
		t.Fatalf("content mapping: call=%s result=%s name=%s results=%d thinking=%d notes=%d", callID, resultID, resultName, toolResults, thinking, notes)
	}
}

func TestDiscoveryRejectsDataAndLinksSubagents(t *testing.T) {
	// Given: data JSONL and two transcripts in one project, including a subagent.
	root := t.TempDir()
	dir := filepath.Join(root, "Project", "imports")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"data.jsonl":  `{"type":"user","message":{"role":"user"},"uuid":"data"}`,
		"main.jsonl":  fixture,
		"agent.jsonl": strings.ReplaceAll(fixture, `"sessionId":"old"`, `"sessionId":"old","isSidechain":true`),
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// When: scanning the entire root.
	sources, scanned, err := discover(root)
	if err != nil {
		t.Fatal(err)
	}
	// Then: data is excluded and the subagent names and references its parent.
	if scanned != 3 || len(sources) != 2 {
		t.Fatalf("scan=%d sessions=%d", scanned, len(sources))
	}
	for _, s := range sources {
		if s.CWD != filepath.Join(root, "Project") {
			t.Fatal("wrong project cwd")
		}
		if s.Sidechain {
			data, err := convert(s, "/sessions")
			if err != nil {
				t.Fatal(err)
			}
			header := decoded(t, data)[0]
			if s.ParentName == "" || !strings.Contains(s.Name, s.ParentName) ||
				header["parentSession"] != filepath.Join("/sessions", s.ParentPath) {
				t.Fatal("subagent parent link missing")
			}
		}
	}
}

func TestImportIsIdempotentAndNeverOverwritesContinuedSession(t *testing.T) {
	// Given: an already imported session with an appended local continuation.
	root, sessions := t.TempDir(), t.TempDir()
	path := filepath.Join(root, "session.jsonl")
	if err := os.WriteFile(path, []byte(fixture), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run(root, sessions, false); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(sessions, sessionDir(root), "claude_"+sessionID(path)+".jsonl")
	before, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	continued := append(before, []byte("{\"type\":\"custom\",\"id\":\"continued\"}\n")...)
	if err := os.WriteFile(dest, continued, 0600); err != nil {
		t.Fatal(err)
	}
	// When: reimporting the same source path.
	if err := run(root, sessions, false); err != nil {
		t.Fatal(err)
	}
	// Then: the destination and original source are byte-for-byte unchanged.
	after, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(sessions, "*", "*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, continued) || string(original) != fixture || len(files) != 1 {
		t.Fatal("reimport changed history, changed source, or duplicated a session")
	}
}

func TestInvalidTranscriptTreeIsRejected(t *testing.T) {
	for _, tc := range []struct{ name, input string }{
		{"cycle", strings.Replace(fixture, `"parentUuid":"summary"`, `"parentUuid":"a"`, 1)},
		{"duplicate", fixture + strings.Split(fixture, "\n")[2] + "\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Given: invalid references in an otherwise recognized transcript.
			s := fixtureSource(t)
			path := filepath.Join(t.TempDir(), "invalid.jsonl")
			if err := os.WriteFile(path, []byte(tc.input), 0600); err != nil {
				t.Fatal(err)
			}
			records, _, _, err := readRecords(path)
			if err != nil {
				t.Fatal(err)
			}
			s.Records = records
			// When: converting the tree.
			_, err = convert(s, "/sessions")
			// Then: no partial session is accepted.
			if err == nil {
				t.Fatal("invalid tree accepted")
			}
		})
	}
}

func TestToolResultsResolveOnlyAlongTheirBranch(t *testing.T) {
	// Given: a result whose call exists only on a sibling branch.
	s := fixtureSource(t)
	s.Records[4].Parent = s.Records[6].Parent
	// When: converting.
	data, err := convert(s, "/sessions")
	if err != nil {
		t.Fatal(err)
	}
	// Then: the orphan is a labelled note, never an invalid executable pair.
	for _, e := range decoded(t, data) {
		if msg, ok := e["message"].(map[string]any); ok && msg["role"] == "toolResult" {
			t.Fatal("matched a call outside the result's ancestry")
		}
	}
}

func TestTranscriptWithMalformedLineFailsClosed(t *testing.T) {
	// Given: a recognized transcript containing a broken record.
	path := filepath.Join(t.TempDir(), "broken.jsonl")
	if err := os.WriteFile(path, []byte(fixture+"{broken\n"), 0600); err != nil {
		t.Fatal(err)
	}
	// When: reading the file.
	_, _, _, err := readRecords(path)
	// Then: the transcript cannot silently lose its broken record.
	if err == nil {
		t.Fatal("malformed transcript accepted")
	}
}

func TestOutOfOrderParentsAreEmittedBeforeChildren(t *testing.T) {
	// Given: a source exporting its child before its parent.
	s := fixtureSource(t)
	s.Records[2], s.Records[3] = s.Records[3], s.Records[2]
	// When: converting the tree.
	data, err := convert(s, "/sessions")
	if err != nil {
		t.Fatal(err)
	}
	// Then: every reference in the output resolves to an earlier entry.
	known := map[string]bool{}
	for _, e := range decoded(t, data)[1:] {
		if p, ok := e["parentId"].(string); ok && !known[p] {
			t.Fatal("unresolved forward reference")
		}
		known[e["id"].(string)] = true
	}
}

func TestDiscoveryDoesNotReadPastThirdLineOfDataFiles(t *testing.T) {
	// Given: a data file whose fourth line would otherwise look like a transcript.
	root := t.TempDir()
	content := "{}\n{}\n{}\n" + fixture
	if err := os.WriteFile(filepath.Join(root, "data.jsonl"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	// When: bounded discovery probes the directory.
	sources, scanned, err := discover(root)
	if err != nil {
		t.Fatal(err)
	}
	// Then: the fourth line cannot cause a full-file read or conversion.
	if scanned != 1 || len(sources) != 0 {
		t.Fatal("discovery exceeded the three-line probe")
	}
}
