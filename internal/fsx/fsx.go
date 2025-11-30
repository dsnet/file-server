// Copyright 2025, Joe Tsai. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE.md file.

// Package fsx extends the [fs] package with write interfaces.
package fsx

import (
	"cmp"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
)

// TODO(https://go.dev/issue/45757): Use standard write interfaces.
// Currently, this locally extends fs.FS to support write operations.

// OpenFlags are flags to use with [OpenFileFS].
type OpenFlags int

const (
	// Exactly one of [ReadOnly], [WriteOnly], or [ReadWrite] must be specified.
	ReadOnly  OpenFlags = OpenFlags(os.O_RDONLY) // open the file read-only.
	WriteOnly OpenFlags = OpenFlags(os.O_WRONLY) // open the file write-only.
	ReadWrite OpenFlags = OpenFlags(os.O_RDWR)   // open the file read-write.

	// The remaining values may be or'ed in to control behavior.
	CreateFile      OpenFlags = OpenFlags(os.O_CREATE) // create a new file if none exists.
	CreateExclusive OpenFlags = OpenFlags(os.O_EXCL)   // used with O_CREATE, file must not exist.
	TruncateFile    OpenFlags = OpenFlags(os.O_TRUNC)  // truncate regular writable file when opened.
	AppendWrites    OpenFlags = OpenFlags(os.O_APPEND) // append data to the file when writing.
	SyncWrites      OpenFlags = OpenFlags(os.O_SYNC)   // open for synchronous I/O.
)

// OpenFileFS is the interface implemented by a file system
// that supports opening files with explicit flags and permissions.
//
// OpenFile opens the named file with the specified flag ([ReadOnly], [ReadWrite],
// [CreateFile], etc.) and permission bits (before umask). Unlike Open, OpenFile
// can create new files when used with [CreateFile] and can open files for writing.
// If successful, methods on the returned file can be used for I/O.
// If there is an error, it will be of type [fs.PathError].
type OpenFileFS interface {
	OpenFile(name string, flag OpenFlags, perm fs.FileMode) (fs.File, error)
}

// OpenFile opens the named file in the filesystem with the specified flags
// ([ReadOnly], [WriteOnly], [ReadWrite], [CreateFile], etc.) and permission bits.
// It returns an error if the filesystem does not implement [OpenFileFS],
// or if the open operation fails.
// If there is an error, it will be of type [fs.PathError].
func OpenFile(fsys fs.FS, name string, flag OpenFlags, perm fs.FileMode) (fs.File, error) {
	fsys2, ok := fsys.(OpenFileFS)
	if !ok {
		return nil, &fs.PathError{Op: "openfile", Path: name, Err: fs.ErrInvalid}
	}
	return fsys2.OpenFile(name, flag, perm)
}

// WriteFile writes data to the named file in the filesystem, creating it if necessary.
// If the file already exists, WriteFile truncates it before writing.
// It returns an error if the filesystem does not implement [OpenFileFS],
// if the opened file does not support writing, or if the write operation fails.
// If there is an error, it will be of type [fs.PathError].
func WriteFile(fsys fs.FS, name string, data []byte, perm fs.FileMode) error {
	f, err := OpenFile(fsys, name, WriteOnly|CreateFile|TruncateFile, perm)
	if err != nil {
		return err
	}
	defer f.Close()
	w, ok := f.(io.Writer)
	if !ok {
		return &fs.PathError{Op: "writefile", Path: name, Err: fs.ErrInvalid}
	}
	_, err = w.Write(data)
	return cmp.Or(err, f.Close())
}

// MakeDirFS is the interface implemented by a file system
// that supports creating directories.
//
// MakeDir creates a new directory with the specified name and permission
// bits (before umask). If there is an error, it will be of type [fs.PathError].
type MakeDirFS interface {
	MakeDir(name string, perm fs.FileMode) error
}

// MakeDir creates a new directory in the filesystem with the specified name
// and permission bits (before umask).
// It returns an error if the filesystem does not implement [MakeDirFS],
// or if the directory creation fails.
// If there is an error, it will be of type [fs.PathError].
func MakeDir(fsys fs.FS, name string, perm fs.FileMode) error {
	fsys2, ok := fsys.(MakeDirFS)
	if !ok {
		return &fs.PathError{Op: "makedir", Path: name, Err: fs.ErrInvalid}
	}
	return fsys2.MakeDir(name, perm)
}

// RenameFS is the interface implemented by a file system
// that supports renaming files and directories.
//
// Rename renames (moves) oldName to newName. If newName already exists and
// is not a directory, Rename replaces it. OS-specific restrictions may apply
// when oldName and newName are in different directories.
// If there is an error, it will be of type [LinkError].
type RenameFS interface {
	Rename(oldName, newName string) error
}

// LinkError records an error during a link operation involving two paths,
// such as Rename. It wraps the underlying error and includes both the old
// and new path names for debugging purposes.
//
// This is an alias for [os.LinkError].
type LinkError = os.LinkError

// Rename renames (moves) a file or directory from oldName to newName in the filesystem.
// If newName already exists and is not a directory, Rename replaces it.
// It returns an error if the filesystem does not implement [RenameFS],
// or if the rename operation fails.
// If there is an error, it will be of type [LinkError].
func Rename(fsys fs.FS, oldName, newName string) error {
	fsys2, ok := fsys.(RenameFS)
	if !ok {
		return &LinkError{Op: "rename", Old: oldName, New: newName, Err: fs.ErrInvalid}
	}
	return fsys2.Rename(oldName, newName)
}

// RemoveFS is the interface implemented by a file system
// that supports removing files and empty directories.
//
// Remove removes the named file or (empty) directory.
// If there is an error, it will be of type [fs.PathError].
type RemoveFS interface {
	Remove(name string) error
}

// Remove removes the named file or empty directory from the filesystem.
// It returns an error if the filesystem does not implement [RemoveFS],
// or if the removal fails.
// If there is an error, it will be of type [fs.PathError].
func Remove(fsys fs.FS, name string) error {
	fsys2, ok := fsys.(RemoveFS)
	if !ok {
		return &fs.PathError{Op: "remove", Path: name, Err: fs.ErrInvalid}
	}
	return fsys2.Remove(name)
}

// RemoveAll removes path and any children it contains.
// It removes everything it can but returns the first error it encounters.
// If the path does not exist, RemoveAll returns nil (no error).
// If there is an error, it will be of type [fs.PathError].
func RemoveAll(fsys fs.FS, name string) error {
	// Check if the file or folder even exists.
	fi, err := fs.Stat(fsys, name)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}

	// Remove all files in the folder (if necessary).
	var firstErr error
	if fi.IsDir() {
		fes, err := fs.ReadDir(fsys, name)
		if err != nil {
			return err
		}
		for _, fe := range fes {
			childName := path.Join(name, fe.Name())
			firstErr = cmp.Or(firstErr, RemoveAll(fsys, childName))
		}
	}

	// Remove the file (or hopefully empty folder).
	return cmp.Or(firstErr, Remove(fsys, name))
}
