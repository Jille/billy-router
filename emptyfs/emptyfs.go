// Package emptyfs provides a read-only filesystem that always remains empty.
// It is most useful as the root filesystem for a billy-router.
package emptyfs

import (
	"os"
	"path"
	"time"

	"github.com/go-git/go-billy/v5"
)

func New() billy.Filesystem {
	return Filesystem{time.Now()}
}

type Filesystem struct {
	ctime time.Time
}

func (Filesystem) Join(elem ...string) string {
	return path.Join(elem...)
}

func (Filesystem) Create(p string) (billy.File, error) {
	return nil, os.ErrPermission
}

func (Filesystem) Open(p string) (billy.File, error) {
	return nil, os.ErrPermission
}

func (Filesystem) OpenFile(p string, flag int, mode os.FileMode) (billy.File, error) {
	return nil, os.ErrPermission
}

func (Filesystem) Rename(from, to string) error {
	return os.ErrPermission
}

func (Filesystem) Remove(p string) error {
	return os.ErrPermission
}

func (Filesystem) ReadDir(p string) ([]os.FileInfo, error) {
	switch p {
	case "/", "", ".":
		return nil, nil
	default:
		return nil, os.ErrNotExist
	}
}

func (Filesystem) MkdirAll(p string, perm os.FileMode) error {
	return os.ErrPermission
}

func (Filesystem) Symlink(target, link string) error {
	return os.ErrPermission
}

func (Filesystem) Readlink(p string) (string, error) {
	return "", os.ErrNotExist
}

func (Filesystem) Chmod(p string, mode os.FileMode) error {
	return os.ErrPermission
}

func (Filesystem) Chown(p string, uid, gid int) error {
	return os.ErrPermission
}

func (Filesystem) Lchown(p string, uid, gid int) error {
	return os.ErrPermission
}

func (Filesystem) Chtimes(p string, atime, mtime time.Time) error {
	return os.ErrPermission
}

func (f Filesystem) Chroot(p string) (billy.Filesystem, error) {
	return nil, os.ErrNotExist
}

func (Filesystem) TempFile(dir, prefix string) (billy.File, error) {
	return nil, os.ErrPermission
}

func (Filesystem) Root() string {
	return "/"
}

func (f Filesystem) Stat(p string) (os.FileInfo, error) {
	switch p {
	case "/", "", ".":
		return rootDir{f.ctime}, nil
	default:
		return nil, os.ErrNotExist
	}
}

func (f Filesystem) Lstat(p string) (os.FileInfo, error) {
	return f.Stat(p)
}

type rootDir struct {
	ctime time.Time
}

func (rootDir) Name() string {
	return ""
}

func (rootDir) Size() int64 {
	return 4096
}

func (rootDir) Mode() os.FileMode {
	return 0555 | os.ModeDir
}

func (v rootDir) ModTime() time.Time {
	return v.ctime
}

func (rootDir) IsDir() bool {
	return true
}

func (rootDir) Sys() interface{} {
	return nil
}
