# billy-router

[![GoDoc](https://godoc.org/github.com/Jille/billy-router?status.svg)](https://godoc.org/github.com/Jille/billy-router)

This library provides a virtual billy.Filesystem backed by other filesystems. You create it with a root filesystem, and then mount/overlay other filesystems over it.

For example, you could use an in-memory tempfs for /tmp and use emptyfs to keep /var/empty empty:

```golang
func main() {
	root := osfs.New("/")
	r := router.New(root)
	r.Mount("/tmp", memfs.New())
	r.Mount("/var/empty", emptyfs.New())
}
```

Directories don't have to exist to be mounted on. In fact, any intermediate directory will implicitly be created (and removed when Umounted).
