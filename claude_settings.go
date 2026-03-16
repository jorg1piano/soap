package main

import "encoding/json"

// ClaudeHook is a single hook command entry.
type ClaudeHook struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

// ClaudeHookGroup is a group of hooks with an optional matcher.
type ClaudeHookGroup struct {
	Matcher string       `json:"matcher,omitempty"`
	Hooks   []ClaudeHook `json:"hooks"`
}

// ClaudeSettings represents the structure of a Claude settings file.
type ClaudeSettings struct {
	Hooks  map[string][]ClaudeHookGroup `json:"hooks,omitempty"`
	Extras map[string]json.RawMessage   `json:"-"`
}

func (s *ClaudeSettings) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	if hooksRaw, ok := raw["hooks"]; ok {
		if err := json.Unmarshal(hooksRaw, &s.Hooks); err != nil {
			return err
		}
		delete(raw, "hooks")
	}

	s.Extras = raw
	return nil
}

func (s ClaudeSettings) MarshalJSON() ([]byte, error) {
	raw := make(map[string]json.RawMessage)
	for k, v := range s.Extras {
		raw[k] = v
	}

	if s.Hooks != nil {
		hooksJSON, err := json.Marshal(s.Hooks)
		if err != nil {
			return nil, err
		}
		raw["hooks"] = hooksJSON
	}

	return json.Marshal(raw)
}

// mergeHooks merges new hook groups into existing settings, deduplicating entries.
func mergeHooks(existing, new *ClaudeSettings) *ClaudeSettings {
	if existing.Hooks == nil {
		existing.Hooks = make(map[string][]ClaudeHookGroup)
	}

	for event, newGroups := range new.Hooks {
		existingGroups := existing.Hooks[event]
		for _, ng := range newGroups {
			if !containsHookGroup(existingGroups, ng) {
				existingGroups = append(existingGroups, ng)
			}
		}
		existing.Hooks[event] = existingGroups
	}

	return existing
}

func containsHookGroup(groups []ClaudeHookGroup, target ClaudeHookGroup) bool {
	for _, g := range groups {
		if g.Matcher != target.Matcher || len(g.Hooks) != len(target.Hooks) {
			continue
		}
		match := true
		for i := range g.Hooks {
			if g.Hooks[i] != target.Hooks[i] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
