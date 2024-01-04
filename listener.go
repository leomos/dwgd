package dwgd

import (
	"net"
	"os"
	"path"

	"github.com/docker/go-connections/sockets"
)

const (
	dockerPluginSockDir = "/run/docker/plugins"
	dwgdRunDir          = "/run/dwgd"
	dwgdSockName        = "dwgd.sock"
)

type UnixListener struct {
	sock net.Listener
}

func (u *UnixListener) Accept() (net.Conn, error) {
	return u.sock.Accept()
}

func (u *UnixListener) Close() error {
	err := u.sock.Close()
	if err != nil {
		return err
	}

	os.Remove(path.Join(dockerPluginSockDir, dwgdSockName))
	os.Remove(path.Join(dwgdRunDir, dwgdSockName))

	return nil
}

func (u *UnixListener) Addr() net.Addr {
	return u.sock.Addr()
}

func NewUnixListener() (net.Listener, error) {
	if err := os.MkdirAll(dwgdRunDir, 0777); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(dockerPluginSockDir, 0777); err != nil {
		return nil, err
	}

	fullDwgdSockPath := path.Join(dwgdRunDir, dwgdSockName)
	listener, err := sockets.NewUnixSocket(fullDwgdSockPath, 0)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(fullDwgdSockPath, 0777); err != nil {
		return nil, err
	}

	dockerPluginSockPath := path.Join(dockerPluginSockDir, dwgdSockName)
	err = os.Symlink(fullDwgdSockPath, dockerPluginSockPath)
	if err != nil {
		return nil, err
	}

	return &UnixListener{
		sock: listener,
	}, nil
}
