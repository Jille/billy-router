package router

import (
	"fmt"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/helper/chroot"
	"github.com/go-git/go-billy/v5/helper/polyfill"
)

type Router struct {
	mtx                           sync.RWMutex
	routes                        subRoute
	crossFilesystemRenameCallback func(fromFS, toFS billy.Filesystem, fromPath, toPath string) error
}

var _ billy.Filesystem = &Router{}
var _ billy.Change = &Router{}

type subRoute struct {
	fs     billy.Filesystem
	mtime  time.Time
	routes map[string]*subRoute
}

func New(root billy.Basic) *Router {
	return &Router{
		routes: subRoute{
			fs:     polyfill.New(root),
			mtime:  time.Now(),
			routes: map[string]*subRoute{},
		},
		crossFilesystemRenameCallback: crossFilesystemRenameNotSupported,
	}
}

func crossFilesystemRenameNotSupported(fromFS, toFS billy.Filesystem, fromPath, toPath string) error {
	return fmt.Errorf("renaming across virtual mountpoints is not supported: %w", os.ErrInvalid)
}

func cleanPath(p string) string {
	p = strings.TrimRight(path.Clean(p), "/")
	if p == "." {
		return p
	}
	return ""
}

func (r *Router) Mount(p string, fs billy.Basic) {
	p = cleanPath(p)
	sp := strings.Split(p, "/")
	r.mtx.Lock()
	defer r.mtx.Unlock()
	sr := &r.routes
	now := time.Now()
	for i := 0; len(sp) > i; i++ {
		s, ok := sr.routes[sp[i]]
		if !ok {
			s = &subRoute{
				routes: map[string]*subRoute{},
			}
			sr.routes[sp[i]] = s
		}
		sr = s
		sr.mtime = now
	}
	sr.fs = polyfill.New(fs)
}

func (r *Router) Umount(p string) {
	p = cleanPath(p)
	sp := strings.Split(p, "/")
	r.mtx.Lock()
	defer r.mtx.Unlock()
	parents := make([]*subRoute, len(sp)+1)
	parents[0] = &r.routes
	for i := 0; len(sp) > i; i++ {
		s, ok := parents[i].routes[sp[i]]
		if !ok {
			panic(fmt.Errorf("Router.RemoveRoute(%q): not a mountpoint", p))
		}
		parents[i+1] = s
	}
	parents[len(sp)].fs = nil
	now := time.Now()
	for i := len(sp); i > 0; i-- {
		if len(parents[i].routes) > 0 || parents[i].fs != nil {
			break
		}
		delete(parents[i-1].routes, sp[i-1])
		parents[i-1].mtime = now
	}
}

// SetCrossFilesystemRenameCallback is called to provide an implementation of cross filesystem renaming.
// Renaming cross filesystem is impossible to implement correctly (atomicity can't be done through the billy.Filesystem API).
// However, many people will be okay with an imperfect implementation (like just copy+remove), so you can specify your own implementation.
// The default callback just returns an error.
func (r *Router) SetCrossFilesystemRenameCallback(f func(fromFS, toFS billy.Filesystem, fromPath, toPath string) error) {
	r.mtx.RLock()
	defer r.mtx.RUnlock()
	r.crossFilesystemRenameCallback = f
}

func (r *Router) resolvePath(p string) (string, billy.Filesystem) {
	sub, _, fs := r.resolvePathWithMount(p)
	return sub, fs
}

func (r *Router) resolvePathWithMount(p string) (string, string, billy.Filesystem) {
	p = cleanPath(p)
	sp := strings.Split(p, "/")
	r.mtx.RLock()
	defer r.mtx.RUnlock()
	sr := &r.routes
	lastMount := 0
	mount := sr.fs
	for i := 0; len(sp) > i; i++ {
		s, ok := sr.routes[sp[i]]
		if !ok {
			break
		}
		sr = s
		if sr.fs != nil {
			lastMount = i
		}
	}
	return strings.Join(sp[lastMount:], "/"), strings.Join(sp[:lastMount], "/"), mount
}

func (r *Router) Capabilities() billy.Capability {
	r.mtx.RLock()
	root := r.routes.fs
	r.mtx.RUnlock()
	return billy.Capabilities(root)
}

func (r *Router) Join(elem ...string) string {
	return path.Join(elem...)
}

func (r *Router) Create(p string) (billy.File, error) {
	sub, fs := r.resolvePath(p)
	fh, err := fs.Create(sub)
	if err != nil {
		return nil, err
	}
	return wrappedFile{fh, p}, nil
}
func (r *Router) Open(p string) (billy.File, error) {
	sub, fs := r.resolvePath(p)
	fh, err := fs.Open(sub)
	if err != nil {
		return nil, err
	}
	return wrappedFile{fh, p}, nil
}
func (r *Router) OpenFile(p string, flag int, mode os.FileMode) (billy.File, error) {
	sub, fs := r.resolvePath(p)
	fh, err := fs.OpenFile(sub, flag, mode)
	if err != nil {
		return nil, err
	}
	return wrappedFile{fh, p}, nil
}
func (r *Router) Stat(p string) (os.FileInfo, error) {
	sub, fs := r.resolvePath(p)
	return fs.Stat(sub)
}
func (r *Router) Rename(from, to string) error {
	fromSub, fromMount, fromFS := r.resolvePathWithMount(from)
	toSub, toMount, toFS := r.resolvePathWithMount(to)
	if fromFS == toFS && fromMount == toMount {
		return fromFS.Rename(fromSub, toSub)
	}
	return r.crossFilesystemRenameCallback(fromFS, toFS, fromSub, toSub)
}
func (r *Router) Remove(p string) error {
	sub, fs := r.resolvePath(p)
	return fs.Remove(sub)
}
func (r *Router) ReadDir(p string) ([]os.FileInfo, error) {
	sub, fs := r.resolvePath(p)
	sp := strings.Split(p, "/")
	r.mtx.RLock()
	sr := &r.routes
	for i := 0; len(sp) > i; i++ {
		s, ok := sr.routes[sp[i]]
		if !ok {
			break
		}
		sr = s
	}
	if len(sr.routes) == 0 {
		r.mtx.RUnlock()
		return fs.ReadDir(sub)
	}
	routeCopy := make(map[string]*subRoute, len(sr.routes))
	for mp, ro := range sr.routes {
		routeCopy[mp] = ro
	}
	r.mtx.RUnlock()
	entries, err := fs.ReadDir(sub)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		entries = nil
	}
	out := make(map[string]os.FileInfo, len(entries)+len(routeCopy))
	for _, e := range entries {
		out[e.Name()] = e
	}
	for mp, ro := range routeCopy {
		if ro.fs == nil {
			if e, found := out[mp]; found && e.IsDir() {
				continue
			}
			out[mp] = virtualDir{mp, ro.mtime}
		} else {
			out[mp], err = ro.fs.Stat("/")
			if err != nil {
				return nil, err
			}
		}
	}
	names := make([]string, 0, len(out))
	for fn := range out {
		names = append(names, fn)
	}
	sort.Strings(names)
	ret := make([]os.FileInfo, len(names))
	for i, fn := range names {
		ret[i] = out[fn]
	}
	return ret, nil
}

type virtualDir struct {
	name  string
	mtime time.Time
}

func (v virtualDir) Name() string {
	return v.name
}

func (virtualDir) Size() int64 {
	return 4096
}

func (virtualDir) Mode() os.FileMode {
	return 0755 | os.ModeDir
}

func (v virtualDir) ModTime() time.Time {
	return v.mtime
}

func (v virtualDir) IsDir() bool {
	return true
}

func (v virtualDir) Sys() interface{} {
	return nil
}

func (r *Router) MkdirAll(p string, perm os.FileMode) error {
	sub, fs := r.resolvePath(p)
	return fs.MkdirAll(sub, perm)
}
func (r *Router) Symlink(target, link string) error {
	return billy.ErrNotSupported
}
func (r *Router) Readlink(p string) (string, error) {
	sub, fs := r.resolvePath(p)
	return fs.Readlink(sub)
}
func (r *Router) Lstat(p string) (os.FileInfo, error) {
	sub, fs := r.resolvePath(p)
	return fs.Lstat(sub)
}
func (r *Router) Chmod(p string, mode os.FileMode) error {
	sub, fs := r.resolvePath(p)
	if cfs, ok := fs.(billy.Change); ok {
		return cfs.Chmod(sub, mode)
	}
	return billy.ErrNotSupported
}
func (r *Router) Chown(p string, uid, gid int) error {
	sub, fs := r.resolvePath(p)
	if cfs, ok := fs.(billy.Change); ok {
		return cfs.Chown(sub, uid, gid)
	}
	return billy.ErrNotSupported
}
func (r *Router) Lchown(p string, uid, gid int) error {
	sub, fs := r.resolvePath(p)
	if cfs, ok := fs.(billy.Change); ok {
		return cfs.Lchown(sub, uid, gid)
	}
	return billy.ErrNotSupported
}
func (r *Router) Chtimes(p string, atime, mtime time.Time) error {
	sub, fs := r.resolvePath(p)
	if cfs, ok := fs.(billy.Change); ok {
		return cfs.Chtimes(sub, atime, mtime)
	}
	return billy.ErrNotSupported
}
func (r *Router) Chroot(p string) (billy.Filesystem, error) {
	sub, fs := r.resolvePath(p)
	if _, err := fs.Lstat(sub); err != nil {
		return nil, err
	}
	return chroot.New(r, p), nil
}
func (r *Router) TempFile(dir, prefix string) (billy.File, error) {
	if dir == "" {
		r.mtx.RLock()
		root := r.routes.fs
		r.mtx.RUnlock()
		return root.TempFile(dir, prefix)
	}
	sub, fs := r.resolvePath(dir)
	return fs.TempFile(sub, prefix)
}
func (r *Router) Root() string {
	r.mtx.RLock()
	root := r.routes.fs
	r.mtx.RUnlock()
	return root.Root()
}

type wrappedFile struct {
	billy.File
	originalName string
}

func (f wrappedFile) Name() string {
	return f.originalName
}
