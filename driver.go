package dwgd

import (
	"errors"
	"fmt"
	"net"
	"os"

	"github.com/docker/go-plugins-helpers/network"
	_ "github.com/mattn/go-sqlite3"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type wgController interface {
	Device(name string) (*wgtypes.Device, error)
	ConfigureDevice(name string, cfg wgtypes.Config) error
}

// Docker WireGuard Driver
type Driver struct {
	network.Driver

	c   commander
	wgc wgController
	s   *Storage
}

func NewDriver(dbPath string, c commander, wgc wgController) (*Driver, error) {
	if c == nil {
		c = &execCommander{}
	}

	path, err := c.LookPath("ip")
	if err != nil {
		TraceLog.Printf("Couldn't find 'ip' utility: %s", err)
	} else {
		TraceLog.Printf("Using 'ip' utility at the following path: %s", path)
	}

	if wgc == nil {
		wgc, err = wgctrl.New()
		if err != nil {
			return nil, err
		}
	}

	s := &Storage{}
	err = s.Open(dbPath)
	if err != nil {
		return nil, err
	}

	return &Driver{
		c:   c,
		wgc: wgc,
		s:   s,
	}, nil
}

func (d *Driver) Close() error {
	return d.s.Close()
}

func (d *Driver) GetCapabilities() (*network.CapabilitiesResponse, error) {
	TraceLog.Printf("GetCapabilities\n")
	return &network.CapabilitiesResponse{Scope: network.LocalScope, ConnectivityScope: network.LocalScope}, nil
}

func (d *Driver) CreateNetwork(r *network.CreateNetworkRequest) error {
	TraceLog.Printf("CreateNetwork: %+v\n", Jsonify(r))
	var err error

	n := &Network{}
	m := r.Options["com.docker.network.generic"].(map[string]interface{})

	// The following two ifs are used to discern whether we are working in
	// ifname mode or pubkey mode.
	// By default we expect to work in pubkey mode, which is why if the ifname
	// parameter is not present we do not return an error.
	var iface *wgtypes.Device
	ifname, ok := m["dwgd.ifname"].(string)
	if !ok {
		n.ifname = ""
	} else {
		iface, err = d.wgc.Device(ifname)
		if errors.Is(err, os.ErrNotExist) {
			TraceLog.Printf("Interface %s not recognized\n", ifname)
			return err
		}
		TraceLog.Printf("Using %s as the WireGuard server interface\n", iface.Name)
		n.ifname = iface.Name
	}

	if iface != nil {
		n.pubkey = iface.PublicKey
	} else {
		payload, ok := m["dwgd.pubkey"].(string)
		if !ok {
			return fmt.Errorf("dwgd.pubkey option missing")
		}
		n.pubkey, err = wgtypes.ParseKey(payload)
		if err != nil {
			return err
		}
	}

	// From this point on we get all the other parameters needed for both modes.
	endpoint, ok := m["dwgd.endpoint"].(string)
	if !ok {
		if iface != nil {
			endpoint = fmt.Sprintf("localhost:%d", iface.ListenPort)
		} else {
			return fmt.Errorf("dwgd.endpoint option missing")
		}
	}
	n.endpoint, err = net.ResolveUDPAddr("udp", endpoint)
	if err != nil {
		return err
	}

	seed, ok := m["dwgd.seed"].(string)
	if !ok {
		return fmt.Errorf("dwgd.seed option missing")
	}
	n.seed = []byte(seed)

	route, ok := m["dwgd.route"].(string)
	if !ok {
		route = ""
	}
	n.route = route

	n.id = r.NetworkID
	return d.s.AddNetwork(n)
}

func (d *Driver) DeleteNetwork(r *network.DeleteNetworkRequest) error {
	TraceLog.Printf("DeleteNetwork: %+v\n", Jsonify(r))
	return d.s.RemoveNetwork(r.NetworkID)
}

func (d *Driver) CreateEndpoint(r *network.CreateEndpointRequest) (*network.CreateEndpointResponse, error) {
	TraceLog.Printf("CreateEndpoint: %+v\n", Jsonify(r))

	n, err := d.s.GetNetwork(r.NetworkID)
	if err != nil {
		return nil, err
	}
	if n == nil {
		return nil, fmt.Errorf("NetworkID %s not found", r.NetworkID)
	}

	c, err := d.s.GetClient(r.EndpointID)
	if err != nil {
		return nil, err
	}
	if c != nil {
		return nil, fmt.Errorf("EndpointID %s already exists", r.EndpointID)
	}

	ip, _, err := net.ParseCIDR(r.Interface.Address)
	if err != nil {
		return nil, err
	}

	endpointIdMaxLen := 12
	if len(r.EndpointID) < 12 {
		endpointIdMaxLen = len(r.EndpointID)
	}
	c = &Client{
		id:      r.EndpointID,
		ip:      ip,
		ifname:  "wg-" + r.EndpointID[:endpointIdMaxLen],
		network: n,
	}

	err = d.s.AddClient(c)
	if err != nil {
		return nil, err
	}

	return &network.CreateEndpointResponse{}, nil
}

func (d *Driver) DeleteEndpoint(r *network.DeleteEndpointRequest) error {
	TraceLog.Printf("DeleteEndpoint: %+v\n", Jsonify(r))
	c, err := d.s.GetClient(r.EndpointID)
	if err != nil {
		return err
	}
	if c == nil {
		return fmt.Errorf("EndpointID %s not found", r.EndpointID)
	}

	if err := d.c.Run("ip", "link", "delete", c.ifname); err != nil {
		return err
	}

	return d.s.RemoveClient(r.EndpointID)
}

func (d *Driver) EndpointInfo(r *network.InfoRequest) (*network.InfoResponse, error) {
	TraceLog.Printf("EndpointInfo: %+v\n", Jsonify(r))
	return &network.InfoResponse{Value: make(map[string]string)}, nil
}

func (d *Driver) Join(r *network.JoinRequest) (*network.JoinResponse, error) {
	TraceLog.Printf("Join: %+v\n", Jsonify(r))

	c, err := d.s.GetClient(r.EndpointID)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, fmt.Errorf("EndpointID %s not found", r.EndpointID)
	}

	if err := d.c.Run("ip", "link", "add", "name", c.ifname, "type", "wireguard"); err != nil {
		return nil, err
	}

	cfg := c.Config()

	err = d.wgc.ConfigureDevice(c.ifname, cfg)
	if err != nil {
		return nil, err
	}

	if c.network.ifname != "" {
		TraceLog.Printf("Adding peer to: %s\n", c.network.ifname)
		iface, err := d.wgc.Device(c.network.ifname)
		if err != nil {
			return nil, err
		}

		peers := make([]wgtypes.PeerConfig, 1)
		peers[0] = c.PeerConfig()

		newNetworkIfaceCfg := wgtypes.Config{
			PrivateKey:   &iface.PrivateKey,
			ListenPort:   &iface.ListenPort,
			FirewallMark: &iface.FirewallMark,
			ReplacePeers: false,
			Peers:        peers,
		}
		TraceLog.Printf("Updating configuration for %s:\n%+v\n", iface.Name, newNetworkIfaceCfg)

		err = d.wgc.ConfigureDevice(iface.Name, newNetworkIfaceCfg)
		if err != nil {
			return nil, err
		}
	}

	err = moveToRootlessNamespaceIfNecessary(d.c, r.SandboxKey, c.ifname)
	if err != nil {
		return nil, err
	}

	staticRoutes := make([]*network.StaticRoute, 0)
	if c.network.route != "" {
		staticRoutes = append(staticRoutes, &network.StaticRoute{
			Destination: c.network.route,
			RouteType:   1,
		})
	}

	return &network.JoinResponse{
		InterfaceName: network.InterfaceName{
			SrcName:   c.ifname,
			DstPrefix: "wg",
		},
		StaticRoutes:          staticRoutes,
		DisableGatewayService: true,
	}, nil
}

func (d *Driver) Leave(r *network.LeaveRequest) error {
	TraceLog.Printf("Leave: %+v\n", Jsonify(r))

	c, err := d.s.GetClient(r.EndpointID)
	if err != nil {
		return err
	}
	if c == nil {
		return fmt.Errorf("EndpointID %s not found", r.EndpointID)
	}

	if c.network.ifname != "" {
		TraceLog.Printf("Removing peer from: %s\n", c.network.ifname)
		iface, err := d.wgc.Device(c.network.ifname)
		if err != nil {
			return err
		}

		peers := make([]wgtypes.PeerConfig, 1)
		clientPeer := c.PeerConfig()
		clientPeer.Remove = true
		peers[0] = clientPeer

		newNetworkIfaceCfg := wgtypes.Config{
			PrivateKey:   &iface.PrivateKey,
			ListenPort:   &iface.ListenPort,
			FirewallMark: &iface.FirewallMark,
			ReplacePeers: false,
			Peers:        peers,
		}
		TraceLog.Printf("Updating configuration for %s:\n%+v\n", iface.Name, Jsonify(newNetworkIfaceCfg))

		err = d.wgc.ConfigureDevice(iface.Name, newNetworkIfaceCfg)
		if err != nil {
			return err
		}
	}

	return nil
}
