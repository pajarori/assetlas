package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteIfChangedSkipsWhenIdentical(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	content := []byte("hello world\n")
	if err := WriteIfChanged(path, content); err != nil {
		t.Fatal(err)
	}
	info1, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteIfChanged(path, content); err != nil {
		t.Fatal(err)
	}
	info2, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info1.ModTime().Equal(info2.ModTime()) {
		t.Errorf("expected ModTime unchanged on identical write")
	}
}

func TestWriteIfChangedRewritesWhenDifferent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	if err := WriteIfChanged(path, []byte("first")); err != nil {
		t.Fatal(err)
	}
	if err := WriteIfChanged(path, []byte("second")); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "second" {
		t.Errorf("expected %q, got %q", "second", got)
	}
}

func TestWriteIfChangedCreatesParent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	if err := WriteIfChanged(path, []byte("x")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file created, got %v", err)
	}
}
