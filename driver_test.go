package dwgd

import (
	"fmt"
	"io/fs"
	"testing"

	"github.com/docker/go-plugins-helpers/network"
	"github.com/google/go-cmp/cmp"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func DeviceFixture() *wgtypes.Device {
	network := NetworkFixture()
	return &wgtypes.Device{
		Name:       "dwgd0",
		ListenPort: network.endpoint.Port,
		PublicKey:  network.pubkey,
	}
}

type testCommander struct {
	ChmodFunc    func(name string, mode fs.FileMode) error
	MkdirAllFunc func(path string, perm fs.FileMode) error
	ReadDirFunc  func(name string) ([]fs.DirEntry, error)
	ReadFileFunc func(name string) ([]byte, error)
	RemoveFunc   func(name string) error
	SymlinkFunc  func(oldname string, newname string) error
	LookPathFunc func(file string) (string, error)
	RunFunc      func(name string, arg ...string) error
	RunHistory   [][]string
}

func (t *testCommander) Chmod(name string, mode fs.FileMode) error {
	return t.ChmodFunc(name, mode)
}

func (t *testCommander) MkdirAll(path string, perm fs.FileMode) error {
	return t.MkdirAllFunc(path, perm)
}

func (t *testCommander) ReadDir(name string) ([]fs.DirEntry, error) {
	return t.ReadDirFunc(name)
}

func (t *testCommander) ReadFile(name string) ([]byte, error) {
	return t.ReadFileFunc(name)
}

func (t *testCommander) Remove(name string) error {
	return t.RemoveFunc(name)
}

func (t *testCommander) Symlink(oldname string, newname string) error {
	return t.SymlinkFunc(oldname, newname)
}

func (t *testCommander) LookPath(file string) (string, error) {
	return t.LookPathFunc(file)
}

func (t *testCommander) Run(name string, arg ...string) error {
	return t.RunFunc(name, arg...)
}

func CommanderFixture() *testCommander {
	t := &testCommander{}
	t.ChmodFunc = func(name string, mode fs.FileMode) error {
		return nil
	}
	t.MkdirAllFunc = func(path string, perm fs.FileMode) error {
		return nil
	}
	t.ReadDirFunc = func(name string) ([]fs.DirEntry, error) {
		return []fs.DirEntry{}, nil
	}
	t.ReadFileFunc = func(name string) ([]byte, error) {
		return []byte{}, nil
	}
	t.RemoveFunc = func(name string) error {
		return nil
	}
	t.SymlinkFunc = func(oldname, newname string) error {
		return nil
	}
	t.LookPathFunc = func(file string) (string, error) {
		return file, nil
	}
	t.RunFunc = func(name string, arg ...string) error {
		if t.RunHistory == nil {
			t.RunHistory = make([][]string, 0)
		}
		fullCommand := append([]string{name}, arg...)
		t.RunHistory = append(t.RunHistory, fullCommand)
		return nil
	}
	return t
}

type testWgController struct {
	ConfigureDeviceFunc func(name string, cfg wgtypes.Config) error
	DeviceFunc          func(name string) (*wgtypes.Device, error)
	Devices             map[string]*wgtypes.Device
}

// ConfigureDevice implements wgController.
func (t *testWgController) ConfigureDevice(name string, cfg wgtypes.Config) error {
	return t.ConfigureDeviceFunc(name, cfg)
}

// Device implements wgController.
func (t *testWgController) Device(name string) (*wgtypes.Device, error) {
	return t.DeviceFunc(name)
}

func WgControllerFixture() *testWgController {
	wgc := &testWgController{
		Devices: make(map[string]*wgtypes.Device),
	}

	wgc.ConfigureDeviceFunc = func(name string, cfg wgtypes.Config) error {
		return nil
	}
	wgc.DeviceFunc = func(name string) (*wgtypes.Device, error) {
		df := DeviceFixture()
		if name != df.Name {
			return nil, fmt.Errorf("device %s does not exist", name)
		}
		return df, nil
	}
	return wgc
}

func TestDriver(t *testing.T) {
	d, err := NewDriver(DbPathFixture(), CommanderFixture(), WgControllerFixture())
	if err != nil {
		t.Fatal(err)
	}
	err = d.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func MustCreateNetwork(t *testing.T, d *Driver, ifnameMode bool) *Network {
	net := NetworkFixture()
	options := map[string]interface{}{
		"dwgd.seed":     string(net.seed),
		"dwgd.endpoint": net.endpoint.String(),
		"dwgd.route":    net.route,
	}
	if ifnameMode {
		options["dwgd.ifname"] = net.ifname
	} else {
		options["dwgd.pubkey"] = net.pubkey.String()
		net.ifname = ""
	}

	err := d.CreateNetwork(&network.CreateNetworkRequest{
		NetworkID: net.id,
		Options: map[string]interface{}{
			"com.docker.network.generic": options,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	other, err := d.s.GetNetwork(net.id)
	if err != nil {
		t.Fatal(err)
	}
	if !cmp.Equal(net, other, cmp.AllowUnexported(Network{})) {
		t.Fatalf("mismatch: %#v != %#v", net, other)
	}

	return other
}

func MustCreateEndpoint(t *testing.T, d *Driver) *Client {
	net := NetworkFixture()
	client := ClientFixture(net)
	_, err := d.CreateEndpoint(&network.CreateEndpointRequest{
		NetworkID:  net.id,
		EndpointID: client.id,
		Interface: &network.EndpointInterface{
			Address: fmt.Sprintf("%s/32", client.ip.String()),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	other, err := d.s.GetClient(client.id)
	if err != nil {
		t.Fatal(err)
	}
	if !cmp.Equal(client, other, cmp.AllowUnexported(Network{}), cmp.AllowUnexported(Client{})) {
		t.Fatalf("mismatch: %#v != %#v", net, other)
	}

	return other
}

func TestDriver_CreateNetwork(t *testing.T) {
	t.Run("ifname mode", func(t *testing.T) {
		d, err := NewDriver(DbPathFixture(), CommanderFixture(), WgControllerFixture())
		if err != nil {
			t.Fatal(err)
		}
		defer d.Close()
		MustCreateNetwork(t, d, true)
	})

	t.Run("pubkey mode", func(t *testing.T) {
		d, err := NewDriver(DbPathFixture(), CommanderFixture(), WgControllerFixture())
		if err != nil {
			t.Fatal(err)
		}
		defer d.Close()
		MustCreateNetwork(t, d, false)
	})
}

func TestDriver_DeleteNetwork(t *testing.T) {
	t.Run("ifname mode", func(t *testing.T) {
		d, err := NewDriver(DbPathFixture(), CommanderFixture(), WgControllerFixture())
		if err != nil {
			t.Fatal(err)
		}
		defer d.Close()

		net := MustCreateNetwork(t, d, true)

		err = d.DeleteNetwork(&network.DeleteNetworkRequest{
			NetworkID: net.id,
		})
		if err != nil {
			t.Fatal(err)
		}

		n, err := d.s.GetNetwork(net.id)
		if err != nil {
			t.Fatal(err)
		}
		if n != nil {
			t.Fatalf("mismatch: nil != %#v", n)
		}
	})

	t.Run("pubkey mode", func(t *testing.T) {
		d, err := NewDriver(DbPathFixture(), CommanderFixture(), WgControllerFixture())
		if err != nil {
			t.Fatal(err)
		}
		defer d.Close()

		net := MustCreateNetwork(t, d, false)

		err = d.DeleteNetwork(&network.DeleteNetworkRequest{
			NetworkID: net.id,
		})
		if err != nil {
			t.Fatal(err)
		}

		n, err := d.s.GetNetwork(net.id)
		if err != nil {
			t.Fatal(err)
		}
		if n != nil {
			t.Fatalf("mismatch: nil != %#v", n)
		}
	})
}

func TestDriver_CreateEndpoint(t *testing.T) {
	d, err := NewDriver(DbPathFixture(), CommanderFixture(), WgControllerFixture())
	if err != nil {
		t.Fatal(err)
	}

	MustCreateNetwork(t, d, true)
	MustCreateEndpoint(t, d)
}

func TestDriver_DeleteEndpoint(t *testing.T) {
	d, err := NewDriver(DbPathFixture(), CommanderFixture(), WgControllerFixture())
	if err != nil {
		t.Fatal(err)
	}

	net := MustCreateNetwork(t, d, true)
	client := MustCreateEndpoint(t, d)

	err = d.DeleteEndpoint(&network.DeleteEndpointRequest{
		NetworkID:  net.id,
		EndpointID: client.id,
	})
	if err != nil {
		t.Fatal(err)
	}

	other, err := d.s.GetClient(client.id)
	if err != nil {
		t.Fatal(err)
	}
	if other != nil {
		t.Fatalf("mismatch: nil != %#v", other)
	}
}

func TestDriver_Join(t *testing.T) {
	t.Run("non rootless", func(t *testing.T) {
		tc := CommanderFixture()
		d, err := NewDriver(DbPathFixture(), tc, WgControllerFixture())
		if err != nil {
			t.Fatal(err)
		}

		net := MustCreateNetwork(t, d, true)
		client := MustCreateEndpoint(t, d)

		_, err = d.Join(&network.JoinRequest{
			NetworkID:  net.id,
			EndpointID: client.id,
			SandboxKey: "/foo/bar",
		})
		if err != nil {
			t.Fatal(err)
		}

		expectedHistory := [][]string{
			{"ip", "link", "add", "name", client.ifname, "type", "wireguard"},
		}
		if !cmp.Equal(tc.RunHistory, expectedHistory) {
			t.Fatalf("mismatch: %#v != %#v", tc.RunHistory, expectedHistory)
		}
	})

	t.Run("rootless", func(t *testing.T) {
		tc := CommanderFixture()
		tc.ReadFileFunc = func(name string) ([]byte, error) { return []byte("1000"), nil }

		d, err := NewDriver(DbPathFixture(), tc, WgControllerFixture())
		if err != nil {
			t.Fatal(err)
		}

		net := MustCreateNetwork(t, d, true)
		client := MustCreateEndpoint(t, d)

		_, err = d.Join(&network.JoinRequest{
			NetworkID:  net.id,
			EndpointID: client.id,
			SandboxKey: "/run/user/1000",
		})
		if err != nil {
			t.Fatal(err)
		}

		expectedHistory := [][]string{
			{"ip", "link", "add", "name", client.ifname, "type", "wireguard"},
			{"ip", "link", "set", client.ifname, "netns", "1000"},
		}
		if !cmp.Equal(tc.RunHistory, expectedHistory) {
			t.Fatalf("mismatch: %#v != %#v", tc.RunHistory, expectedHistory)
		}
	})
}

func TestDriver_Leave(t *testing.T) {
	tc := CommanderFixture()
	wgc := WgControllerFixture()

	d, err := NewDriver(DbPathFixture(), tc, wgc)
	if err != nil {
		t.Fatal(err)
	}

	net := MustCreateNetwork(t, d, true)
	client := MustCreateEndpoint(t, d)

	_, err = d.Join(&network.JoinRequest{
		NetworkID:  net.id,
		EndpointID: client.id,
		SandboxKey: "/foo/bar",
	})
	if err != nil {
		t.Fatal(err)
	}

	err = d.Leave(&network.LeaveRequest{
		NetworkID:  net.id,
		EndpointID: client.id,
	})
	if err != nil {
		t.Fatal(err)
	}

	expectedHistory := [][]string{
		{"ip", "link", "add", "name", client.ifname, "type", "wireguard"},
	}
	if !cmp.Equal(tc.RunHistory, expectedHistory) {
		t.Fatalf("mismatch: %#v != %#v", tc.RunHistory, expectedHistory)
	}
}
