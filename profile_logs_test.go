package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfileLogsAreBoundedScopedAndReadBothGenerations(t *testing.T) {
	setTestStorage(t)
	a := multiTestApp(t)
	dir := applicationLogDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "engine.log")
	if err := os.WriteFile(path+".old", []byte("old profile=\"a\" entry\nold profile=\"b\" other\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("new profile=\"a\" entry\nnew profile=\"aa\" other\n"), 0600); err != nil {
		t.Fatal(err)
	}
	lines, err := a.GetProfileLogs("a")
	if err != nil || len(lines) != 2 || !strings.HasPrefix(lines[0], "old") || !strings.HasPrefix(lines[1], "new") {
		t.Fatal("generation/identity mix", lines, err)
	}
	var data strings.Builder
	for i := 0; i < 240; i++ {
		fmt.Fprintf(&data, "profile=\"a\" sequence=%d\n", i)
	}
	if err := os.WriteFile(path, []byte(data.String()), 0600); err != nil {
		t.Fatal(err)
	}
	lines, err = a.GetProfileLogs("a")
	if err != nil || len(lines) != 200 || !strings.Contains(lines[0], "sequence=40") || !strings.Contains(lines[199], "sequence=239") {
		t.Fatal("log window lost the newest entries", err)
	}
	if _, err = a.GetProfileLogs("unknown"); err == nil {
		t.Fatal("unknown configuration must not read logs")
	}
}
