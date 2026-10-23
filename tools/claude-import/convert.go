package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// JSON objects are only used at the two external transcript format boundaries.
type object = map[string]any

func convert(s source, sessions string) ([]byte, error) {
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	start := time.Unix(0, 0).UTC()
	if len(s.Records) > 0 {
		start = recordTime(s.Records[0])
	}
	header := object{"type": "session", "version": 3, "id": s.ID, "cwd": s.CWD, "timestamp": start.Format(time.RFC3339Nano),
		"claudeImport": object{"source": s.Path, "sha256": s.Hash}}
	if s.ParentPath != "" {
		header["parentSession"] = filepath.Join(sessions, s.ParentPath)
	}
	if err := encoder.Encode(header); err != nil {
		return nil, err
	}
	ids := make(map[string]int)
	for i, r := range s.Records {
		if r.UUID != "" {
			if _, exists := ids[r.UUID]; exists {
				return nil, fmt.Errorf("duplicate record UUID in %s", s.Path)
			}
			ids[r.UUID] = i
		}
	}
	// A record can expand into multiple OMO entries. Children attach to its tail.
	tails := make(map[int]string)
	visiting := make(map[int]bool)
	var emit func(int) error
	emit = func(index int) error {
		if _, done := tails[index]; done {
			return nil
		}
		if visiting[index] {
			return fmt.Errorf("cyclic parent tree in %s", s.Path)
		}
		visiting[index] = true
		r := s.Records[index]
		var parent any
		if r.Parent != nil {
			if p, found := ids[*r.Parent]; found {
				if err := emit(p); err != nil {
					return err
				}
				parent = tails[p]
			}
		}
		ts := recordTime(r)
		parts, err := convertRecord(r, toolNames(s.Records, ids, index))
		if err != nil {
			return fmt.Errorf("record %d in %s: %w", index, s.Path, err)
		}
		if len(parts) == 0 {
			parts = []object{{"type": "custom", "customType": "claude-import-record",
				"data": json.RawMessage(r.Raw)}}
		}
		for j, entry := range parts {
			id := digest(fmt.Sprintf("%s:%d:%d", s.ID, index, j))[:16]
			entry["id"], entry["parentId"], entry["timestamp"] = id, parent, ts.Format(time.RFC3339Nano)
			entry["claudeImport"] = object{"uuid": r.UUID, "parentUuid": r.Parent, "cwd": r.CWD}
			if err := encoder.Encode(entry); err != nil {
				return err
			}
			parent = id
		}
		tails[index] = parent.(string)
		delete(visiting, index)
		return nil
	}
	for i := range s.Records {
		if err := emit(i); err != nil {
			return nil, err
		}
	}
	var leaf any
	if len(s.Records) > 0 {
		leaf = tails[len(s.Records)-1]
	}
	originalCWDs := make(map[string]bool)
	for _, r := range s.Records {
		if r.CWD != "" {
			originalCWDs[r.CWD] = true
		}
	}
	if err := encoder.Encode(object{"type": "custom", "id": digest(s.ID + ":metadata")[:16], "parentId": leaf,
		"timestamp": start.Format(time.RFC3339Nano), "customType": "claude-import",
		"data": object{"source": s.Path, "sha256": s.Hash, "originalCWDs": originalCWDs, "parentName": s.ParentName,
			"historicalToolsOnly": true}}); err != nil {
		return nil, err
	}
	if err := encoder.Encode(object{"type": "session_info", "id": digest(s.ID + ":name")[:16],
		"parentId": digest(s.ID + ":metadata")[:16], "timestamp": start.Format(time.RFC3339Nano), "name": s.Name}); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func toolNames(records []record, ids map[string]int, index int) map[string]string {
	names := make(map[string]string)
	seen := make(map[int]bool)
	for !seen[index] {
		seen[index] = true
		r := records[index]
		if r.Message != nil {
			var blocks []block
			if json.Unmarshal(r.Message.Content, &blocks) == nil {
				for _, b := range blocks {
					if b.Type == "tool_use" {
						// The closest ancestor wins when an ID is reused on another branch.
						if _, exists := names[b.ID]; !exists {
							names[b.ID] = b.Name
						}
					}
				}
			}
		}
		if r.Parent == nil {
			break
		}
		next, exists := ids[*r.Parent]
		if !exists {
			break
		}
		index = next
	}
	return names
}

func note(text string) object {
	return object{"type": "custom_message", "customType": "claude-import-note", "content": text, "display": true}
}

func convertRecord(r record, names map[string]string) ([]object, error) {
	if r.CompactSummary {
		if r.Message == nil {
			return []object{note("[Claude compaction summary]\n" + string(r.Raw))}, nil
		}
		return []object{note("[Claude compaction summary]\n" + textContent(r.Message.Content))}, nil
	}
	if r.Type == "attachment" {
		return []object{note("[Claude attachment; not executed]\n" + string(r.Attachment))}, nil
	}
	if len(r.Compact) > 0 {
		return []object{note("[Claude compaction metadata]\n" + string(r.Raw))}, nil
	}
	if r.Message == nil || (r.Type != "user" && r.Type != "assistant") {
		if r.Type == "summary" {
			return []object{note("[Claude summary]\n" + string(r.Raw))}, nil
		}
		return nil, nil
	}
	ts := recordTime(r).UnixMilli()
	var blocks []block
	var text string
	if json.Unmarshal(r.Message.Content, &text) == nil {
		blocks = []block{{Type: "text", Text: text}}
	} else if err := json.Unmarshal(r.Message.Content, &blocks); err != nil {
		return nil, err
	}
	var entries []object
	var content []object
	flush := func() {
		if len(content) == 0 {
			return
		}
		msg := object{"role": r.Type, "content": content, "timestamp": ts}
		if r.Type == "assistant" {
			stop := "stop"
			for _, b := range content {
				if b["type"] == "toolCall" {
					stop = "toolUse"
				}
			}
			model := r.Message.Model
			if model == "" {
				model = "claude-import"
			}
			msg["api"], msg["provider"], msg["model"], msg["stopReason"] = "anthropic-messages", "anthropic", model, stop
			msg["usage"] = object{"input": 0, "output": 0, "cacheRead": 0, "cacheWrite": 0, "totalTokens": 0,
				"cost": object{"input": 0, "output": 0, "cacheRead": 0, "cacheWrite": 0, "total": 0}}
		}
		entries = append(entries, object{"type": "message", "message": msg})
		content = nil
	}
	for _, b := range blocks {
		switch b.Type {
		case "text":
			content = append(content, object{"type": "text", "text": b.Text})
		case "thinking":
			if r.Type == "assistant" {
				content = append(content, object{"type": "thinking", "thinking": b.Thinking})
			}
		case "tool_use":
			args := b.Input
			if len(args) == 0 || string(args) == "null" {
				args = json.RawMessage("{}")
			}
			content = append(content, object{"type": "toolCall", "id": b.ID, "name": b.Name, "arguments": args})
		case "tool_result":
			flush()
			name, matched := names[b.ToolUseID]
			if !matched {
				entries = append(entries, note("[Claude orphan tool result "+b.ToolUseID+"]\n"+resultText(b.Content)))
				continue
			}
			entries = append(entries, object{"type": "message", "message": object{"role": "toolResult",
				"toolCallId": b.ToolUseID, "toolName": name, "timestamp": ts, "isError": b.IsError,
				"content": []object{{"type": "text", "text": resultText(b.Content)}}}})
		default:
			// Binary payloads are intentionally represented as notes, never live attachments.
			content = append(content, object{"type": "text", "text": "[Claude attachment/block: " + b.Type + "]"})
		}
	}
	flush()
	return entries, nil
}

func resultText(raw json.RawMessage) string {
	if text := textContent(raw); text != "" {
		return text
	}
	return strings.TrimSpace(string(raw))
}
