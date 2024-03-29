package dwgd

import (
	"fmt"
	"net"
	"os/exec"

	"github.com/docker/go-plugins-helpers/network"
	_ "github.com/mattn/go-sqlite3"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// Docker WireGuard Driver
type Driver struct {
	network.Driver

	wgc *wgctrl.Client
	s   *Storage
}

func NewDriver(dbPath string) (*Driver, error) {
	path, err := exec.LookPath("ip")
	if err != nil {
		TraceLog.Printf("Couldn't find 'ip' utility: %s", err)
	} else {
		TraceLog.Printf("Using 'ip' utility at the following path: %s", path)
	}

	wgc, err := wgctrl.New()
	if err != nil {
		return nil, err
	}

	s, err := NewStorage(dbPath)
	if err != nil {
		return nil, err
	}

	return &Driver{
		wgc: wgc,
		s:   s,
	}, nil
}

func (d *Driver) Close() error {
	return d.s.db.Close()
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

	endpoint, ok := m["dwgd.endpoint"].(string)
	if !ok {
		return fmt.Errorf("dwgd.endpoint option missing")
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

	payload, ok := m["dwgd.pubkey"].(string)
	if !ok {
		return fmt.Errorf("dwgd.pubkey option missing")
	}
	n.pubkey, err = wgtypes.ParseKey(payload)
	if err != nil {
		return err
	}

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

	c = &Client{
		id:      r.EndpointID,
		ip:      ip,
		ifname:  "wg-" + r.EndpointID[:12],
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

	cmd := exec.Command("ip", "link", "delete", c.ifname)
	if err := cmd.Run(); err != nil {
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

	cmd := exec.Command("ip", "link", "add", "name", c.ifname, "type", "wireguard")
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	cfg, err := c.Config()
	if err != nil {
		return nil, err
	}

	err = d.wgc.ConfigureDevice(c.ifname, cfg)
	if err != nil {
		return nil, err
	}

	moveToRootlessNamespaceIfNecessary(r.SandboxKey, c.ifname)

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

	return nil
}
