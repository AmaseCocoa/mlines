package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeManifest(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "mlines.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLoadManifestShell(t *testing.T) {
	dir := writeManifest(t, `{
		"description": "test",
		"script_root": "./scripts",
		"scripts": {
			"gi": {"type": "shell_script", "description": "d", "path": "gi", "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}
		}
	}`)
	m, err := loadManifest(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if m.Scripts["gi"].Path != "gi" {
		t.Errorf("path not parsed")
	}
}

func TestLoadManifestDefaultScriptRoot(t *testing.T) {
	dir := writeManifest(t, `{"scripts": {"a": {"type": "shell_script", "path": "a"}}}`)
	m, err := loadManifest(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if m.ScriptRoot != defaultScriptRoot {
		t.Errorf("got %q want %q", m.ScriptRoot, defaultScriptRoot)
	}
}

func TestValidateErrors(t *testing.T) {
	cases := []struct {
		name string
		json string
	}{
		{"empty scripts", `{"scripts": {}}`},
		{"shell without path", `{"scripts": {"a": {"type": "shell_script"}}}`},
		{"binary without bin/registry", `{"scripts": {"a": {"type": "binary"}}}`},
		{"binary with both", `{"scripts": {"a": {"type": "binary", "bin": {"x-y": {"file": "https://e/f"}}, "registry": "https://e/r"}}}`},
		{"unknown type", `{"scripts": {"a": {"type": "dmg"}}}`},
		{"bad sha", `{"scripts": {"a": {"type": "shell_script", "path": "a", "sha256": "zz"}}}`},
		{"abs path", `{"scripts": {"a": {"type": "shell_script", "path": "/abs"}}}`},
		{"bad name", `{"scripts": {"a/b": {"type": "shell_script", "path": "x"}}}`},
	}
	for _, tc := range cases {
		dir := writeManifest(t, tc.json)
		if _, err := loadManifest(dir); err == nil {
			t.Errorf("%s: エラーが期待されたが成功した", tc.name)
		}
	}
}

func TestParseScriptRef(t *testing.T) {
	repo, name, err := parseScriptRef("cocoa-gists/gi")
	if err != nil || repo != "cocoa-gists" || name != "gi" {
		t.Errorf("got %q %q %v", repo, name, err)
	}
	if _, _, err := parseScriptRef("nonslash"); err == nil {
		t.Errorf("スラッシュなしはエラーになるべき")
	}
}

func TestVerifyShellScript(t *testing.T) {
	dir := t.TempDir()
	scriptDir := filepath.Join(dir, "scripts")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "#!/bin/sh\necho hi\n"
	if err := os.WriteFile(filepath.Join(scriptDir, "s"), []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	sum, err := sha256File(filepath.Join(scriptDir, "s"))
	if err != nil {
		t.Fatal(err)
	}
	m := &Manifest{ScriptRoot: "./scripts", Scripts: map[string]ScriptEntry{
		"s": {Type: "shell_script", Path: "s", Sha256: sum},
	}}
	if err := verifyShellScript(m, dir, "s"); err != nil {
		t.Errorf("一致するはず: %v", err)
	}
	m.Scripts["s"] = ScriptEntry{Type: "shell_script", Path: "s", Sha256: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}
	if err := verifyShellScript(m, dir, "s"); err == nil {
		t.Errorf("不一致はエラーになるべき")
	}
}
