package dwgd

import (
	"net"
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
	c    commander
}

func (u *UnixListener) Accept() (net.Conn, error) {
	return u.sock.Accept()
}

func (u *UnixListener) Close() error {
	err := u.sock.Close()
	if err != nil {
		return err
	}

	u.c.Remove(path.Join(dockerPluginSockDir, dwgdSockName))
	u.c.Remove(path.Join(dwgdRunDir, dwgdSockName))

	return nil
}

func (u *UnixListener) Addr() net.Addr {
	return u.sock.Addr()
}

func NewUnixListener(c commander) (net.Listener, error) {
	if c == nil {
		c = &execCommander{}
	}

	if err := c.MkdirAll(dwgdRunDir, 0777); err != nil {
		return nil, err
	}

	if err := c.MkdirAll(dockerPluginSockDir, 0755); err != nil {
		return nil, err
	}

	fullDwgdSockPath := path.Join(dwgdRunDir, dwgdSockName)
	listener, err := sockets.NewUnixSocket(fullDwgdSockPath, 0)
	if err != nil {
		return nil, err
	}
	if err := c.Chmod(fullDwgdSockPath, 0777); err != nil {
		return nil, err
	}

	dockerPluginSockPath := path.Join(dockerPluginSockDir, dwgdSockName)
	err = c.Symlink(fullDwgdSockPath, dockerPluginSockPath)
	if err != nil {
		return nil, err
	}

	return &UnixListener{
		sock: listener,
		c:    c,
	}, nil
}
