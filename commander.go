package dwgd

import (
	"io/fs"
	"os"
	"os/exec"
)

// commander abstracts the os and os/exec stdlib packages.
// This is needed to mock in unit tests.
type commander interface {
	// os
	Chmod(name string, mode fs.FileMode) error
	MkdirAll(name string, perm fs.FileMode) error
	ReadFile(name string) ([]byte, error)
	ReadDir(name string) ([]fs.DirEntry, error)
	Remove(name string) error
	Symlink(oldname string, newname string) error
	// os/exec
	LookPath(file string) (string, error)
	Run(name string, arg ...string) error
}

type execCommander struct{}

func (e *execCommander) Chmod(name string, mode fs.FileMode) error {
	return os.Chmod(name, mode)
}

func (e *execCommander) MkdirAll(path string, perm fs.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (e *execCommander) ReadDir(name string) ([]fs.DirEntry, error) {
	return os.ReadDir(name)
}

func (e *execCommander) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}

func (e *execCommander) Remove(name string) error {
	return os.Remove(name)
}

func (e *execCommander) Symlink(oldname string, newname string) error {
	return os.Symlink(oldname, newname)
}

func (e *execCommander) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func (e *execCommander) Run(name string, arg ...string) error {
	cmd := exec.Command(name, arg...)
	return cmd.Run()
}
