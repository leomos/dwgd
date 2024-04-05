package dwgd

import (
	"crypto/sha256"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"sort"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type Network struct {
	id       string
	endpoint *net.UDPAddr
	seed     []byte
	pubkey   wgtypes.Key
	route    string
	ifname   string
}

func (n *Network) PeerConfig() wgtypes.PeerConfig {
	keepalive := 25 * time.Second

	_, ipnet, _ := net.ParseCIDR("0.0.0.0/0")
	allowedIPs := []net.IPNet{*ipnet}

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

func (c *Client) Config() wgtypes.Config {
	privkey := GeneratePrivateKey(c.network.seed, c.ip)

	peers := make([]wgtypes.PeerConfig, 1)
	peers[0] = c.network.PeerConfig()

	return wgtypes.Config{
		PrivateKey: privkey,
		Peers:      peers,
	}
}

func (c *Client) PeerConfig() wgtypes.PeerConfig {
	keepalive := 25 * time.Second

	ipnet := net.IPNet{
		IP:   c.ip,
		Mask: []byte{255, 255, 255, 255},
	}
	allowedIPs := []net.IPNet{ipnet}

	privkey := GeneratePrivateKey(c.network.seed, c.ip)

	return wgtypes.PeerConfig{
		PublicKey:                   privkey.PublicKey(),
		Remove:                      false,
		UpdateOnly:                  false,
		PresharedKey:                nil,
		Endpoint:                    nil,
		PersistentKeepaliveInterval: &keepalive,
		ReplaceAllowedIPs:           true,
		AllowedIPs:                  allowedIPs,
	}
}

func GeneratePrivateKey(seed []byte, ip net.IP) *wgtypes.Key {
	h := sha256.New()
	h.Write(seed)
	h.Write(ip)

	// since the size of a SHA256 checksum is 32 bytes by default,
	// wgtypes.NewKey cannot return error
	priv, _ := wgtypes.NewKey(h.Sum(nil))

	// Modify random bytes using algorithm described at:
	// https://cr.yp.to/ecdh.html.
	priv[0] &= 248
	priv[31] &= 127
	priv[31] |= 64

	return &priv
}

type Storage struct {
	db *sql.DB
}

func (s *Storage) Open(path string) error {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return err
	}
	s.db = db

	// Enable foreign key checks.
	if _, err := db.Exec(`PRAGMA foreign_keys = ON;`); err != nil {
		return fmt.Errorf("foreign keys pragma: %w", err)
	}

	if err := s.migrate(); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	return err
}

func (s *Storage) Close() error {
	return s.db.Close()
}

//go:embed migrations/*.sql
var migrationFS embed.FS

// migrate sets up migration tracking and executes pending migration files.
//
// Migration files are embedded in the sqlite/migration folder and are executed
// in lexigraphical order.
//
// Once a migration is run, its name is stored in the 'migrations' table so it
// is not re-executed. Migrations run in a transaction to prevent partial
// migrations.
func (s *Storage) migrate() error {
	// Ensure the 'migrations' table exists so we don't duplicate migrations.
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS migrations (name TEXT PRIMARY KEY);`); err != nil {
		return fmt.Errorf("cannot create migrations table: %w", err)
	}

	// Read migration files from our embedded file system.
	// This uses Go 1.16's 'embed' package.
	names, err := fs.Glob(migrationFS, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)

	// Loop over all migration files and execute them in order.
	for _, name := range names {
		if err := s.migrateFile(name); err != nil {
			return fmt.Errorf("migration error: name=%q err=%w", name, err)
		}
	}
	return nil
}

// migrate runs a single migration file within a transaction. On success, the
// migration file name is saved to the "migrations" table to prevent re-running.
func (s *Storage) migrateFile(name string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Ensure migration has not already been run.
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM migrations WHERE name = ?`, name).Scan(&n); err != nil {
		return err
	} else if n != 0 {
		return nil // already run migration, skip
	}

	// Read and execute migration file.
	if buf, err := fs.ReadFile(migrationFS, name); err != nil {
		return err
	} else if _, err := tx.Exec(string(buf)); err != nil {
		return err
	}

	// Insert record into migrations to prevent re-running migration.
	if _, err := tx.Exec(`INSERT INTO migrations (name) VALUES (?)`, name); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Storage) AddNetwork(n *Network) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stm, err := s.db.Prepare("INSERT INTO network(id, endpoint, seed, pubkey, route, ifname) VALUES(?, ?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stm.Close()

	r, err := stm.Exec(n.id, n.endpoint.String(), n.seed, n.pubkey[:], n.route, n.ifname)
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

	return tx.Commit()
}

func (s *Storage) RemoveNetwork(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stm, err := tx.Prepare("DELETE FROM network WHERE id = ?")
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

	return tx.Commit()
}

func (s *Storage) GetNetwork(id string) (*Network, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("SELECT id, endpoint, seed, pubkey, route, ifname FROM network WHERE id = ?")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	n := &Network{}
	var endpoint string
	var pubkey []byte

	err = stmt.QueryRow(id).Scan(&n.id, &endpoint, &n.seed, &pubkey, &n.route, &n.ifname)
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
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stm, err := tx.Prepare("INSERT INTO client(id, network_id, ip, ifname) VALUES(?, ?, ?, ?)")
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

	return tx.Commit()
}

func (s *Storage) RemoveClient(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stm, err := tx.Prepare("DELETE FROM client WHERE id = ?")
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

	return tx.Commit()
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
	network.route,
	network.ifname
FROM 
	client 
INNER JOIN network
ON client.network_id = network.id
WHERE client.id = ?
`
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(q)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	c := &Client{}
	c.network = &Network{}
	var endpoint string
	var ip string
	var pubkey []byte
	err = stmt.QueryRow(id).Scan(&c.id, &c.network.id, &ip, &c.ifname, &endpoint, &c.network.seed, &pubkey, &c.network.route, &c.network.ifname)
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
