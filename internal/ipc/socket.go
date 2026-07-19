package ipc

import (
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// sockaddr_un caps socket paths (~104 bytes on darwin). Save directories can
// live anywhere, so long paths bind/dial via a brief chdir into the socket's
// directory using its short basename. chdir is process-wide: the dance is
// serialized and restores cwd immediately. Nothing else in the daemon or CLI
// relies on cwd at these moments (all internal paths are absolute).
const maxSockPath = 100

var chdirMu sync.Mutex

func withSockDir(path string, fn func(shortPath string) error) error {
	if len(path) <= maxSockPath {
		return fn(path)
	}
	chdirMu.Lock()
	defer chdirMu.Unlock()
	oldWd, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := os.Chdir(filepath.Dir(path)); err != nil {
		return err
	}
	defer os.Chdir(oldWd)
	return fn(filepath.Base(path))
}

func listenUnix(path string) (net.Listener, error) {
	var ln net.Listener
	err := withSockDir(path, func(p string) error {
		var e error
		ln, e = net.Listen("unix", p)
		return e
	})
	return ln, err
}

func dialUnix(path string, timeout time.Duration) (net.Conn, error) {
	var conn net.Conn
	err := withSockDir(path, func(p string) error {
		var e error
		conn, e = net.DialTimeout("unix", p, timeout)
		return e
	})
	return conn, err
}
