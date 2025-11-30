// Copyright 2025, Joe Tsai. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE.md file.

package fsx

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

type dirFS string

// Dir constructs an [fs.FS] rooted at the specified path.
//
// The result is guaranteed to additionally implement:
//   - [fs.StatFS]
//   - [OpenFileFS]
//   - [MakeDirFS]
//   - [RenameFS]
//   - [RemoveFS]
func Dir(root string) fs.FS {
	// TODO: Should we directly return an interface that implements everything?
	// TODO: Support options to avoid operations (e.g., following symlinks)
	// that extend outside the current directory?
	// TODO: Implement [fs.ReadFileFS], [fs.ReadDirFS], [fs.ReadLinkFS].
	return dirFS(root)
}

func (dir dirFS) Stat(name string) (fs.FileInfo, error) {
	fullname, err := dir.join("stat", name)
	if err != nil {
		return nil, err
	}
	f, err := os.Stat(fullname)
	if err != nil {
		return nil, replaceErrorPaths(err, name, name)
	}
	return f, nil
}

func (dir dirFS) Open(name string) (fs.File, error) {
	fullname, err := dir.join("open", name)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(fullname)
	if err != nil {
		return nil, replaceErrorPaths(err, name, name)
	}
	return f, nil
}

func (dir dirFS) OpenFile(name string, flags OpenFlags, perm fs.FileMode) (fs.File, error) {
	fullname, err := dir.join("open", name)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(fullname, int(flags), perm)
	if err != nil {
		return nil, replaceErrorPaths(err, name, name)
	}
	return f, nil
}

func (dir dirFS) MakeDir(name string, perm fs.FileMode) error {
	fullname, err := dir.join("mkdir", name)
	if err != nil {
		return err
	}
	if err := os.Mkdir(fullname, perm); err != nil {
		return replaceErrorPaths(err, name, name)
	}
	return nil
}

func (dir dirFS) Rename(oldName, newName string) error {
	oldFullname, err := dir.join("rename", oldName)
	if err != nil {
		return err
	}
	newFullname, err := dir.join("rename", newName)
	if err != nil {
		return err
	}
	if err := os.Rename(oldFullname, newFullname); err != nil {
		return replaceErrorPaths(err, oldName, newName)
	}
	return nil
}

func (dir dirFS) Remove(name string) error {
	// TODO: Should we be allowed to remove "." itself?
	fullname, err := dir.join("remove", name)
	if err != nil {
		return err
	}
	if err := os.Remove(fullname); err != nil {
		return replaceErrorPaths(err, name, name)
	}
	return nil
}

func (dir dirFS) join(op, name string) (string, error) {
	// TODO: Handle Windows reserved names.
	switch {
	case dir == "":
		return "", &fs.PathError{Op: op, Path: name, Err: errors.New("Dir with empty root")}
	case !fs.ValidPath(name):
		return "", &fs.PathError{Op: op, Path: name, Err: fs.ErrInvalid}
	default:
		return filepath.Join(string(dir), filepath.FromSlash(name)), nil
	}
}

func replaceErrorPaths(err error, oldName, newName string) error {
	var perr *os.PathError
	if errors.As(err, &perr) {
		perr.Path = oldName
	}
	var lerr *os.LinkError
	if errors.As(err, &lerr) {
		lerr.Old = oldName
		lerr.New = newName
	}
	return err
}
