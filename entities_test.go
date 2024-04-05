package dwgd

import (
	"fmt"
	"math"
	"math/rand"
	"net"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func DbPathFixture() string {
	r := rand.Int31n(math.MaxInt32)
	return fmt.Sprintf("file:test.%d?mode=memory&cache=shared", r)
}

func NetworkFixture() *Network {
	endpoint, _ := net.ResolveUDPAddr("udp", "localhost:51820")
	pubkey, _ := wgtypes.ParseKey("BR1A+UneCu1FVBW/zPI/UVKA4gcNMUroj72LwFMMUUs=")
	network := &Network{
		id:       "n1",
		endpoint: endpoint,
		seed:     []byte("supersecretseed"),
		pubkey:   pubkey,
		route:    "0.0.0.0/0",
		ifname:   "dwgd0",
	}
	return network
}

func ClientFixture(network *Network) *Client {
	client := &Client{
		id:      "c1",
		ip:      []byte{10, 0, 0, 2},
		ifname:  "wg-c1",
		network: network,
	}
	return client
}

func MustOpenDB(t *testing.T) *Storage {
	t.Helper()

	s := &Storage{}
	err := s.Open(DbPathFixture())
	if err != nil {
		t.Fatal(err)
	}

	return s
}

func MustCloseDB(t *testing.T, s *Storage) {
	t.Helper()

	err := s.Close()
	if err != nil {
		t.Fatal(err)
	}
}

func MustExistNetwork(t *testing.T, s *Storage, n *Network) {
	t.Helper()

	err := s.AddNetwork(n)
	if err != nil {
		t.Fatal(err)
	}
}

func TestStorage(t *testing.T) {
	s := MustOpenDB(t)
	MustCloseDB(t, s)
}

func TestStorage_Network(t *testing.T) {
	network := NetworkFixture()

	t.Run("AddNetwork", func(t *testing.T) {
		s := MustOpenDB(t)
		defer MustCloseDB(t, s)

		err := s.AddNetwork(network)
		if err != nil {
			t.Fatal(err)
		}

		other, err := s.GetNetwork(network.id)
		if err != nil {
			t.Fatal(err)
		}
		if !cmp.Equal(network, other, cmp.AllowUnexported(Network{})) {
			t.Fatalf("mismatch: %#v != %#v", network, other)
		}
	})

	t.Run("RemoveNetwork", func(t *testing.T) {
		s := MustOpenDB(t)
		defer MustCloseDB(t, s)
		err := s.AddNetwork(network)
		if err != nil {
			t.Fatal(err)
		}
		err = s.RemoveNetwork(network.id)
		if err != nil {
			t.Fatal(err)
		}

		other, err := s.GetNetwork(network.id)
		if err != nil {
			t.Fatal(err)
		}
		if other != nil {
			t.Fatalf("mismatch: nil != %#v", other)
		}
	})
}

func TestStorage_Client(t *testing.T) {
	network := NetworkFixture()
	client := ClientFixture(network)

	t.Run("AddClientNotExistingNetwork", func(t *testing.T) {
		s := MustOpenDB(t)
		defer MustCloseDB(t, s)

		err := s.AddClient(client)
		if !strings.Contains(err.Error(), "FOREIGN KEY constraint failed") {
			t.Fatal(err)
		}
	})

	t.Run("AddClient", func(t *testing.T) {
		s := MustOpenDB(t)
		defer MustCloseDB(t, s)
		MustExistNetwork(t, s, network)

		err := s.AddClient(client)
		if err != nil {
			t.Fatal(err)
		}

		other, err := s.GetClient(client.id)
		if err != nil {
			t.Fatal(err)
		}
		if !cmp.Equal(client, other, cmp.AllowUnexported(Client{}), cmp.AllowUnexported(Network{})) {
			t.Fatalf("mismatch: %#v != %#v", client, other)
		}
	})

	t.Run("RemoveClient", func(t *testing.T) {
		s := MustOpenDB(t)
		defer MustCloseDB(t, s)
		MustExistNetwork(t, s, network)

		err := s.AddClient(client)
		if err != nil {
			t.Fatal(err)
		}
		err = s.RemoveClient(client.id)
		if err != nil {
			t.Fatal(err)
		}
		other, err := s.GetClient(client.id)
		if err != nil {
			t.Fatal(err)
		}
		if other != nil {
			t.Fatalf("mismatch: nil != %#v", other)
		}
	})

	t.Run("RemoveNetwork", func(t *testing.T) {
		// removing the network associated with the client should remove the
		// client too

		s := MustOpenDB(t)
		defer MustCloseDB(t, s)
		MustExistNetwork(t, s, network)

		err := s.AddClient(client)
		if err != nil {
			t.Fatal(err)
		}

		err = s.RemoveNetwork(network.id)
		if err != nil {
			t.Fatal(err)
		}

		other, err := s.GetClient(client.id)
		if err != nil {
			t.Fatal(err)
		}
		if other != nil {
			t.Fatalf("mismatch: nil != %#v", other)
		}
	})
}
