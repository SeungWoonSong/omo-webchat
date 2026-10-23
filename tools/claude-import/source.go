package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type record struct {
	Type           string          `json:"type"`
	UUID           string          `json:"uuid"`
	Parent         *string         `json:"parentUuid"`
	SessionID      string          `json:"sessionId"`
	CWD            string          `json:"cwd"`
	Timestamp      string          `json:"timestamp"`
	Sidechain      bool            `json:"isSidechain"`
	CompactSummary bool            `json:"isCompactSummary"`
	Message        *claudeMessage  `json:"message"`
	Attachment     json.RawMessage `json:"attachment"`
	Compact        json.RawMessage `json:"compactMetadata"`
	Raw            json.RawMessage `json:"-"`
}

type claudeMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
	Model   string          `json:"model"`
}

type block struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

type source struct {
	Path, CWD, ID, Hash, Name, SessionID, ParentPath, ParentName string
	Records                                                      []record
	Sidechain                                                    bool
}

func digest(s string) string {
	hash := sha256.Sum256([]byte(s))
	return hex.EncodeToString(hash[:])
}

func sessionID(path string) string {
	h := digest(path)
	return h[:8] + "-" + h[8:12] + "-5" + h[13:16] + "-a" + h[17:20] + "-" + h[20:32]
}

func sessionDir(cwd string) string {
	return "--" + strings.NewReplacer("/", "-", "\\", "-", ":", "-").Replace(strings.TrimLeft(cwd, "/\\")) + "--"
}

// Probe at most three physical lines. Data exports are never read in full.
// A compact boundary is also recognizable because exported main sessions can
// start with metadata before their first user message.
func probeTranscript(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	reader := bufio.NewReader(f)
	for range 3 {
		line, readErr := reader.ReadBytes('\n')
		var r record
		if json.Unmarshal(line, &r) == nil && r.UUID != "" {
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(line, &fields); err != nil {
				return false, err
			}
			_, hasParent := fields["parentUuid"]
			message := r.Message != nil && r.Message.Role == r.Type && (r.Type == "user" || r.Type == "assistant")
			compaction := r.Type == "system" && len(r.Compact) > 0
			if hasParent && (message || compaction) {
				return true, nil
			}
		}
		if readErr == io.EOF {
			return false, nil
		}
		if readErr != nil {
			return false, readErr
		}
	}
	return false, nil
}

func readRecords(path string) ([]record, string, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", false, err
	}
	defer f.Close()
	hash := sha256.New()
	reader := bufio.NewReader(io.TeeReader(f, hash))
	candidate := false
	for {
		line, readErr := reader.ReadBytes('\n')
		// Data exports dwarf the transcripts. Probe only transcript-shaped
		// lines, while reading and hashing every byte of every file.
		if !candidate && bytes.Contains(line, []byte(`"parentUuid"`)) && bytes.Contains(line, []byte(`"uuid"`)) {
			var r record
			if json.Unmarshal(line, &r) == nil {
				if (r.Type == "user" || r.Type == "assistant") && r.Message != nil &&
					r.Message.Role == r.Type && r.UUID != "" {
					candidate = true
				}
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, "", false, readErr
		}
	}
	sum := hex.EncodeToString(hash.Sum(nil))
	if !candidate {
		return nil, sum, false, nil
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, "", false, err
	}
	reader.Reset(f)
	var records []record
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			var r record
			if err := json.Unmarshal(line, &r); err != nil {
				return nil, "", false, fmt.Errorf("invalid JSON in transcript %s: %w", path, err)
			}
			if r.UUID != "" || r.Type == "system" || r.Type == "summary" {
				r.Raw = append(json.RawMessage(nil), line...)
				records = append(records, r)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, "", false, readErr
		}
	}
	return records, sum, true, nil
}

func discover(root string) ([]source, int, error) {
	var sources []source
	scanned := 0
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			return nil
		}
		scanned++
		candidate, err := probeTranscript(path)
		if err != nil || !candidate {
			return err
		}
		records, hash, candidate, err := readRecords(path)
		if err != nil || !candidate {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		project := strings.Split(rel, string(filepath.Separator))[0]
		cwd := root
		if project != filepath.Base(path) {
			cwd = filepath.Join(root, project)
		}
		s := source{Path: path, CWD: cwd, ID: sessionID(path), Hash: hash, Records: records}
		for _, r := range records {
			if r.SessionID != "" && s.SessionID == "" {
				s.SessionID, s.Sidechain = r.SessionID, r.Sidechain
			}
			if s.Name == "" && r.Type == "user" && r.Message != nil {
				s.Name = textContent(r.Message.Content)
			}
		}
		if s.Name == "" {
			s.Name = strings.TrimSuffix(entry.Name(), ".jsonl")
		}
		s.Name = "[Claude] " + truncate(strings.Join(strings.Fields(s.Name), " "), 72)
		sources = append(sources, s)
		return nil
	})
	if err != nil {
		return nil, scanned, err
	}
	for i := range sources {
		if !sources[i].Sidechain {
			continue
		}
		for _, parent := range sources {
			if parent.SessionID == sources[i].SessionID && parent.CWD == sources[i].CWD && !parent.Sidechain {
				sources[i].ParentPath = outputPath(parent, "")
				sources[i].ParentName = parent.Name
				sources[i].Name += " [" + strings.TrimSuffix(filepath.Base(sources[i].Path), ".jsonl") + "; parent: " + parent.Name + "]"
				break
			}
		}
	}
	return sources, scanned, nil
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) > max {
		return string(runes[:max]) + "..."
	}
	return s
}

func recordTime(r record) time.Time {
	t, err := time.Parse(time.RFC3339Nano, r.Timestamp)
	if err != nil {
		return time.Unix(0, 0).UTC()
	}
	return t
}

func outputPath(s source, sessions string) string {
	return filepath.Join(sessions, sessionDir(s.CWD), "claude_"+s.ID+".jsonl")
}

func textContent(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var blocks []block
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}
