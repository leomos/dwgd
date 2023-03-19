package dwgd

import (
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"net"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type Network struct {
	id       string
	endpoint *net.UDPAddr
	seed     []byte
	pubkey   wgtypes.Key
	route    string
}

func (n *Network) PeerConfig() wgtypes.PeerConfig {
	keepalive := 25 * time.Second

	_, ipnet, _ := net.ParseCIDR("0.0.0.0/0")
	allowedIPs := make([]net.IPNet, 1)
	allowedIPs[0] = *ipnet

	return wgtypes.PeerConfig{
		Endpoint:                    n.endpoint,
		PublicKey:                   n.pubkey,
		PersistentKeepaliveInterval: &keepalive,
		AllowedIPs:                  allowedIPs,
		ReplaceAllowedIPs:           true,
	}
}

type Client struct {
	id      string
	ip      net.IP
	ifname  string
	network *Network
}

func (c *Client) Config() (wgtypes.Config, error) {
	privkey, err := GeneratePrivateKey(c.network.seed, c.ip)
	if err != nil {
		return wgtypes.Config{}, err
	}

	peers := make([]wgtypes.PeerConfig, 1)
	peers[0] = c.network.PeerConfig()

	return wgtypes.Config{
		PrivateKey: privkey,
		Peers:      peers,
	}, nil
}

func GeneratePrivateKey(seed []byte, ip net.IP) (*wgtypes.Key, error) {
	h := sha256.New()
	h.Write(seed)
	h.Write(ip)

	priv, err := wgtypes.NewKey(h.Sum(nil))
	if err != nil {
		return nil, err
	}

	// Modify random bytes using algorithm described at:
	// https://cr.yp.to/ecdh.html.
	priv[0] &= 248
	priv[31] &= 127
	priv[31] |= 64

	return &priv, nil
}

var initSql string = `
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS network (
    id TEXT PRIMARY KEY,
    endpoint TEXT,
    seed BLOB,
    pubkey BLOB[32],
	route TEXT
);

CREATE TABLE IF NOT EXISTS client (
    id TEXT PRIMARY KEY,
    network_id TEXT,
    ip TEXT,
    ifname TEXT,

    FOREIGN KEY(network_id) REFERENCES network(id) ON DELETE CASCADE
);
`

type Storage struct {
	db *sql.DB
}

func NewStorage(path string) (*Storage, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(initSql)
	if err != nil {
		return nil, err
	}

	return &Storage{
		db: db,
	}, err
}

func (s *Storage) AddNetwork(n *Network) error {
	stm, err := s.db.Prepare("INSERT INTO network(id, endpoint, seed, pubkey, route) VALUES(?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stm.Close()

	r, err := stm.Exec(n.id, n.endpoint.String(), n.seed, n.pubkey[:], n.route)
	if err != nil {
		return err
	}

	num, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if num != 1 {
		return fmt.Errorf("number of inserted rows: %d is not 1", num)
	}

	return nil
}

func (s *Storage) RemoveNetwork(id string) error {
	stm, err := s.db.Prepare("DELETE FROM network WHERE id = ?")
	if err != nil {
		return err
	}
	defer stm.Close()

	r, err := stm.Exec(id)
	if err != nil {
		return err
	}

	num, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if num != 1 {
		return fmt.Errorf("number of deleted rows: %d is not 1", num)
	}

	return nil
}

func (s *Storage) GetNetwork(id string) (*Network, error) {
	stmt, err := s.db.Prepare("SELECT id, endpoint, seed, pubkey, route FROM network WHERE id = ?")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	n := &Network{}
	var endpoint string
	var pubkey []byte

	err = stmt.QueryRow(id).Scan(&n.id, &endpoint, &n.seed, &pubkey, &n.route)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	n.endpoint, err = net.ResolveUDPAddr("udp", endpoint)
	if err != nil {
		return nil, err
	}
	n.pubkey, err = wgtypes.NewKey(pubkey)
	if err != nil {
		return nil, err
	}

	return n, nil
}

func (s *Storage) AddClient(c *Client) error {
	stm, err := s.db.Prepare("INSERT INTO client(id, network_id, ip, ifname) VALUES(?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stm.Close()

	r, err := stm.Exec(c.id, c.network.id, c.ip.String(), c.ifname)
	if err != nil {
		return err
	}

	num, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if num != 1 {
		return fmt.Errorf("number of inserted rows: %d is not 1", num)
	}

	return nil
}

func (s *Storage) RemoveClient(id string) error {
	stm, err := s.db.Prepare("DELETE FROM client WHERE id = ?")
	if err != nil {
		return err
	}
	defer stm.Close()

	r, err := stm.Exec(id)
	if err != nil {
		return err
	}

	num, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if num != 1 {
		return fmt.Errorf("number of deleted rows: %d is not 1", num)
	}

	return nil
}

func (s *Storage) GetClient(id string) (*Client, error) {
	q := `
SELECT 
	client.id,
	client.network_id,
	client.ip,
	client.ifname,
	network.endpoint,
	network.seed,
	network.pubkey,
	network.route
FROM 
	client 
INNER JOIN network
ON client.network_id = network.id
WHERE client.id = ?
`

	stmt, err := s.db.Prepare(q)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	c := &Client{}
	c.network = &Network{}
	var endpoint string
	var ip string
	var pubkey []byte
	err = stmt.QueryRow(id).Scan(&c.id, &c.network.id, &ip, &c.ifname, &endpoint, &c.network.seed, &pubkey, &c.network.route)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.ip = net.ParseIP(ip)
	c.network.endpoint, err = net.ResolveUDPAddr("udp", endpoint)
	if err != nil {
		return nil, err
	}
	c.network.pubkey, err = wgtypes.NewKey(pubkey)
	if err != nil {
		return nil, err
	}

	return c, nil
}
