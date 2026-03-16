package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// normalize replaces the dynamic executable path with a stable placeholder
// so snapshot comparisons are deterministic.
func normalize(data []byte) string {
	soapPath, err := os.Executable()
	if err != nil {
		return string(data)
	}
	return strings.ReplaceAll(string(data), soapPath, "SOAP_PATH")
}

// snapshot compares got against testdata/<name>.json.
// If the file doesn't exist and -update flag is set, it creates it.
func snapshot(t *testing.T, name string, got []byte) {
	t.Helper()

	// Re-indent for consistent formatting
	var parsed any
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	pretty, _ := json.MarshalIndent(parsed, "", "  ")
	normalized := normalize(pretty) + "\n"

	golden := filepath.Join("testdata", name+".json")

	if os.Getenv("UPDATE_SNAPSHOTS") == "1" {
		os.MkdirAll("testdata", 0755)
		if err := os.WriteFile(golden, []byte(normalized), 0644); err != nil {
			t.Fatalf("writing snapshot: %v", err)
		}
		return
	}

	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("snapshot %s not found (run with UPDATE_SNAPSHOTS=1 to create): %v", golden, err)
	}

	if string(want) != normalized {
		t.Errorf("snapshot mismatch for %s\n\nwant:\n%s\ngot:\n%s", name, string(want), normalized)
	}
}

func TestInstallHooks_FreshInstall(t *testing.T) {
	dir := t.TempDir()

	if err := installHooksInDir(dir); err != nil {
		t.Fatalf("installHooksInDir: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.local.json"))
	if err != nil {
		t.Fatal(err)
	}

	snapshot(t, "fresh_install", data)
}

func TestInstallHooks_MergeWithExisting(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	os.MkdirAll(claudeDir, 0755)

	existing := `{
  "permissions": {
    "allow": ["Read", "Write"]
  },
  "hooks": {
    "CustomEvent": [
      {
        "hooks": [
          {"type": "command", "command": "echo custom"}
        ]
      }
    ]
  }
}`
	os.WriteFile(filepath.Join(claudeDir, "settings.local.json"), []byte(existing), 0644)

	if err := installHooksInDir(dir); err != nil {
		t.Fatalf("installHooksInDir: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.local.json"))
	if err != nil {
		t.Fatal(err)
	}

	snapshot(t, "merge_with_existing", data)
}

func TestInstallHooks_AppendToExistingEvent(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	os.MkdirAll(claudeDir, 0755)

	existing := `{
  "hooks": {
    "SessionStart": [
      {
        "hooks": [
          {"type": "command", "command": "echo my-custom-hook"}
        ]
      }
    ]
  }
}`
	os.WriteFile(filepath.Join(claudeDir, "settings.local.json"), []byte(existing), 0644)

	if err := installHooksInDir(dir); err != nil {
		t.Fatalf("installHooksInDir: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.local.json"))
	if err != nil {
		t.Fatal(err)
	}

	snapshot(t, "append_to_existing_event", data)
}

func TestInstallHooks_DoubleInstall(t *testing.T) {
	dir := t.TempDir()

	if err := installHooksInDir(dir); err != nil {
		t.Fatalf("first install: %v", err)
	}
	if err := installHooksInDir(dir); err != nil {
		t.Fatalf("second install: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.local.json"))
	if err != nil {
		t.Fatal(err)
	}

	snapshot(t, "double_install", data)
}
