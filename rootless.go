package dwgd

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"time"

	"github.com/illarion/gonotify/v2"
)

const (
	xdgRuntimeRoot    = "/run/user/"
	dockerPidFileName = "docker.pid"
)

var (
	userXdgRuntimeDirRegex = regexp.MustCompile(xdgRuntimeRoot + `\d+`)
)

func moveToRootlessNamespace(pid int, ifname string) error {
	TraceLog.Printf("Moving %s to rootless namespace with PID %d\n", ifname, pid)
	cmd := exec.Command("ip", "link", "set", ifname, "netns", fmt.Sprint(pid))
	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}

func moveToRootlessNamespaceIfNecessary(sandboxKey string, ifname string) error {
	match := userXdgRuntimeDirRegex.FindString(sandboxKey)
	if match != "" {
		data, err := os.ReadFile(path.Join(match, dockerPidFileName))
		if err != nil {
			return err
		}

		pid, err := strconv.Atoi(string(data))
		if err != nil {
			return err
		}
		moveToRootlessNamespace(pid, ifname)
	}

	return nil
}

// returns (pid, socket path, errror)
func generateSockSymlinkFromDockerPidFile(dockerPidFileFullPath string) (int, string, error) {
	data, err := os.ReadFile(dockerPidFileFullPath)
	if err != nil {
		return 0, "", err
	}

	pid, err := strconv.Atoi(string(data))
	if err != nil {
		return 0, "", err
	}

	fullDwgdSockPath := path.Join(dwgdRunDir, dwgdSockName)
	dockerPluginSockPath := path.Join(dockerPluginSockDir, dwgdSockName)
	cmd := exec.Command("nsenter", "-U", "-n", "-m", "-t", fmt.Sprint(pid), "ln", "-s", "-f", fullDwgdSockPath, dockerPluginSockPath)
	if err := cmd.Run(); err != nil {
		TraceLog.Printf("Couldn't create symlink on rootless ns (PID: %d): %s\n", pid, err)
		return 0, "", err
	}

	TraceLog.Printf("Created symlink for namespace with PID %d\n", pid)
	return pid, dockerPluginSockPath, nil
}

type RootlessSymlinker struct {
	socketSymlinkPerNs map[int]string
	stopCh             chan int
	inotify            *gonotify.Inotify
}

func NewRootlessSymlinker() (*RootlessSymlinker, error) {
	path, err := exec.LookPath("nsenter")
	if err != nil {
		TraceLog.Printf("Couldn't find 'nsenter' utility: %s", err)
		return nil, err
	} else {
		TraceLog.Printf("Using 'nsenter' utility at the following path: %s", path)
	}

	return &RootlessSymlinker{
		socketSymlinkPerNs: make(map[int]string),
		stopCh:             make(chan int),
	}, nil
}

func (r *RootlessSymlinker) handleEvent(ev gonotify.InotifyEvent) {
	if ev.Mask&(gonotify.IN_CREATE|gonotify.IN_ISDIR) != 0 {
		if !userXdgRuntimeDirRegex.MatchString(ev.Name) {
			return
		}
		r.inotify.AddWatch(ev.Name, gonotify.IN_CLOSE_WRITE)
	} else if ev.Mask&gonotify.IN_CLOSE_WRITE != 0 {
		if !userXdgRuntimeDirRegex.MatchString(ev.Name) {
			return
		}

		TraceLog.Printf("Creating symlink from %s\n", ev.Name)
		retries := 5
		for i := 0; i < retries; i++ {
			pid, sockPath, err := generateSockSymlinkFromDockerPidFile(ev.Name)
			if err == nil {
				r.socketSymlinkPerNs[pid] = sockPath
				return
			}
			TraceLog.Printf("Error during creation of socket symlink: %s\n", err)
			waitSecs := int64(math.Pow(2, float64(i)))
			TraceLog.Printf("[%d/%d] Waiting %d seconds\n", i+1, retries, waitSecs)
			time.Sleep(time.Duration(waitSecs) * time.Second)
		}
	}

}

func (r *RootlessSymlinker) Start() error {
	// We create a context to handle inotify's lifecyle.
	// When the symlinker is stopped we want to stop
	// cleanly also the inotofy instance.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	inotify, err := gonotify.NewInotify(ctx)
	if err != nil {
		return err
	}
	r.inotify = inotify

	// Before starting watching for events we list all the folders
	// in the xdgRuntimeRoot: if there already are some instances
	// of docker rootless running we can handle those
	entries, err := os.ReadDir(xdgRuntimeRoot)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		// If we find any directory whose name is a number
		// we assume that it could be a user's XDG_RUNTIME_DIR
		// We handle this situation creating a "fake" inotify
		// event.
		if !entry.IsDir() {
			continue
		}
		fullPath := path.Join(xdgRuntimeRoot, entry.Name())
		isNumber := userXdgRuntimeDirRegex.MatchString(fullPath)
		if !isNumber {
			continue
		}
		r.handleEvent(gonotify.InotifyEvent{
			Name: fullPath,
			Mask: gonotify.IN_CREATE | gonotify.IN_ISDIR,
		})

		// We also search for a <dockerPidFileName> file
		// inside the directory and handle a constructed event.
		subEntries, err := os.ReadDir(fullPath)
		if err != nil {
			return err
		}
		for _, subEntry := range subEntries {
			if subEntry.Name() == dockerPidFileName {
				r.handleEvent(gonotify.InotifyEvent{
					Name: path.Join(fullPath, subEntry.Name()),
					Mask: gonotify.IN_CLOSE_WRITE,
				})
			}
		}
	}

	err = r.inotify.AddWatch(xdgRuntimeRoot, gonotify.IN_CREATE|gonotify.IN_ISDIR)
	if err != nil {
		return err
	}

	TraceLog.Println("Starting to listen for events")
	for {
		raw, err := r.inotify.ReadDeadline(time.Now().Add(time.Millisecond * 200))
		select {
		case <-r.stopCh:
			return nil
		default:
			{
				if err != nil {
					if err == gonotify.TimeoutError {
						continue
					}
					TraceLog.Printf("Error during inotify reading: %s\n", err)
					return nil
				}

				for _, event := range raw {
					r.handleEvent(event)
				}
			}
		}
	}
}

func (r *RootlessSymlinker) Stop() error {
	r.stopCh <- 0
	close(r.stopCh)

	for pid, path := range r.socketSymlinkPerNs {
		cmd := exec.Command("nsenter", "-U", "-n", "-m", "-t", fmt.Sprint(pid), "rm", "-f", path)
		if err := cmd.Run(); err != nil {
			TraceLog.Printf("Couldn't remove symlink on rootless ns (PID: %d): %s\n", pid, err)
			continue
		}
	}
	return nil
}
