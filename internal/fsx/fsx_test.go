// Copyright 2025, Joe Tsai. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE.md file.

package fsx

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"testing"
)

func TestDir(t *testing.T) {
	tmpDir := t.TempDir()
	fsys := Dir(tmpDir)

	// Test Open and Stat on a non-existent file.
	const noexistFile = "noexist.txt"
	if _, err := fsys.Open(noexistFile); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Open error: %v, want ErrNotExist", err)
	}
	if _, err := fs.Stat(fsys, noexistFile); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Stat error: %v, want ErrNotExist", err)
	}

	// Create a test file.
	const testFile = "test.txt"
	wantData := []byte("hello world")
	f1, err := OpenFile(fsys, testFile, CreateFile|WriteOnly, 0644)
	if err != nil {
		t.Fatalf("OpenFile error: %v", err)
	}
	if _, err := f1.(io.Writer).Write(wantData); err != nil {
		t.Fatalf("File.Write error: %v", err)
	}
	if err := f1.Close(); err != nil {
		t.Fatalf("File.Close error: %v", err)
	}

	// Test Stat on existing file.
	fi, err := fs.Stat(fsys, testFile)
	if err != nil {
		t.Fatalf("Stat error: %v", err)
	}
	if fi.Name() != testFile {
		t.Fatalf("Name = %s, want %v", fi.Name(), testFile)
	}
	if fi.Size() != int64(len(wantData)) {
		t.Fatalf("Size = %d, want %d", fi.Size(), len(wantData))
	}

	// Test Open on existing file.
	f2, err := fsys.Open(testFile)
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	defer f2.Close()
	gotData, err := io.ReadAll(f2)
	if err != nil {
		t.Fatalf("ReadAll error: %v", err)
	}
	if !bytes.Equal(gotData, wantData) {
		t.Fatalf("data mismatch\n\tgot:  %v\n\twant: %v", gotData, wantData)
	}

	// Test creating a directory.
	const testDir = "testdir"
	if err := MakeDir(fsys, testDir, 0775); err != nil {
		t.Fatalf("MakeDir error: %v", err)
	}

	// Verify directory was created
	fi, err = fs.Stat(fsys, testDir)
	if err != nil {
		t.Fatalf("Stat error: %v", err)
	}
	if !fi.IsDir() {
		t.Fatalf("FileInfo.IsDir = false, want true")
	}

	// Test creating directory that already exists.
	if err := MakeDir(fsys, testDir, 0775); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("MakeDir error = %v, want ErrExist", err)
	}

	// Rename a file that does not exist.
	if err := Rename(fsys, noexistFile, "whatever.txt"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Rename error = %v, want ErrNotExist", err)
	}
	if err := Rename(fsys, testFile, "new."+testFile); err != nil {
		t.Fatalf("Rename error: %v", err)
	}

	// Verify old file does not exist.
	if _, err := fs.Stat(fsys, testFile); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Stat error = %v, want ErrNotExist", err)
	}
	// Verify new file does exists.
	if _, err := fs.Stat(fsys, "new."+testFile); err != nil {
		t.Fatalf("Stat error: %v", err)
	}

	// Create another file.
	if err := WriteFile(fsys, path.Join(testDir, "foo"), []byte("fizz buzz"), 0664); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}
	if b, err := fs.ReadFile(fsys, path.Join(testDir, "foo")); err != nil {
		t.Fatalf("ReadFile error: %v", err)
	} else if string(b) != "fizz buzz" {
		t.Fatalf("ReadFile = %q, want %q", b, "fizz buzz")
	}

	// Remove all files.
	if err := RemoveAll(fsys, "."); err != nil {
		t.Fatalf("RemoveAll error: %v", err)
	}
	if _, err := os.Stat(tmpDir); !os.IsNotExist(err) {
		t.Fatalf("Stat error = %v, want ErrNotExist", err)
	}
}
