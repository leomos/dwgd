package dwgd

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"

	"github.com/docker/go-connections/sockets"
)

const (
	dockerPluginSockDir = "/run/docker/plugins"
	dwgdRunDir          = "/run/dwgd"
	dwgdSockName        = "dwgd.sock"
	rootlessSearchPath  = "/run/user/"
)

type UnixListener struct {
	sock               net.Listener
	filesToRemovePerNs map[int][]string
}

func (u *UnixListener) Accept() (net.Conn, error) {
	return u.sock.Accept()
}

func (u *UnixListener) Close() error {
	err := u.sock.Close()
	if err != nil {
		return err
	}

	for _, f := range u.filesToRemovePerNs[0] {
		os.Remove(f)
	}
	delete(u.filesToRemovePerNs, 0)

	for pid, filesToRemove := range u.filesToRemovePerNs {
		for _, f := range filesToRemove {
			cmd := exec.Command("nsenter", "-U", "-n", "-m", "-t", fmt.Sprint(pid), "rm", "-f", f)
			if err := cmd.Run(); err != nil {
				TraceLog.Printf("Couldn't remove symlink on rootless ns (PID: %d): %s\n", pid, err)
				continue
			}
		}
	}

	return nil
}

func (u *UnixListener) Addr() net.Addr {
	return u.sock.Addr()
}

func extrapolatePidFromRootlessDir(p string) (int, error) {
	data, err := os.ReadFile(path.Join(p, "docker.pid"))
	if err != nil {
		return 0, err
	}

	pid, err := strconv.Atoi(string(data))
	if err != nil {
		return 0, err
	}
	return pid, nil
}

func generateSockSymlinksForRootlessNs(rootlessSearchPath string) (map[int][]string, error) {
	fis, err := ioutil.ReadDir(rootlessSearchPath)
	if err != nil {
		return nil, err
	}

	filesToRemovePerNs := make(map[int][]string, 0)
	re := regexp.MustCompile(`\d+`)

	for _, fi := range fis {
		isNumber := re.Match([]byte(fi.Name()))

		if !(isNumber && fi.IsDir()) {
			continue
		}

		pid, err := extrapolatePidFromRootlessDir(path.Join(rootlessSearchPath, fi.Name()))
		if err != nil {
			TraceLog.Printf("Couldn't extrapolate pid from rootless dir: %s\n", err)
			continue
		}

		fullDwgdSockPath := path.Join(dwgdRunDir, dwgdSockName)
		dockerPluginSockPath := path.Join(dockerPluginSockDir, dwgdSockName)
		cmd := exec.Command("nsenter", "-U", "-n", "-m", "-t", fmt.Sprint(pid), "ln", "-s", "-f", fullDwgdSockPath, dockerPluginSockPath)
		if err := cmd.Run(); err != nil {
			TraceLog.Printf("Couldn't create symlink on rootless ns (PID: %d): %s\n", pid, err)
			continue
		}

		filesToRemovePerNs[pid] = []string{dockerPluginSockPath}
	}

	return filesToRemovePerNs, nil
}

func NewUnixListener(rootlessCompatibility bool) (net.Listener, error) {
	filesToRemove := make([]string, 2)

	if err := os.MkdirAll(dwgdRunDir, 0777); err != nil {
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
	filesToRemove = append(filesToRemove, fullDwgdSockPath)

	dockerPluginSockPath := path.Join(dockerPluginSockDir, dwgdSockName)
	err = os.Symlink(fullDwgdSockPath, dockerPluginSockPath)
	if err != nil {
		return nil, err
	}
	filesToRemove = append(filesToRemove, dockerPluginSockPath)

	filesToRemovePerNs := map[int][]string{
		0: filesToRemove,
	}

	if rootlessCompatibility {
		rootlessNsFiles, err := generateSockSymlinksForRootlessNs(rootlessSearchPath)

		if err != nil {
			return nil, err
		}
		for k, v := range rootlessNsFiles {
			TraceLog.Printf("Created unix socket symlink for namespace with PID %d\n", k)
			filesToRemovePerNs[k] = v
		}
	}

	return &UnixListener{
		sock:               listener,
		filesToRemovePerNs: filesToRemovePerNs,
	}, nil
}
