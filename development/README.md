# Development

This folder contains some utilities that can aid the development of `dwgd`:
- `e2e-tests/` contains tests that check the whole `dwgd` lifecycle by creating
the necessary resources (WireGuard interface and docker network), sending a ping
from the container and finally removing everything. The tests can be run like:
`sudo ./development/e2e-tests/test_ifname_mode.sh` and everything should be `OK`.
- `Vagrantfile` a simple Vagrant box that has everything it's needed to run the
`e2e-tests`.

## Developing on local machine

You can develop on your own machine by compiling `dwgd`, creating a WireGuard network and starting `dwgd`:

```sh
go build ./cmd/dwgd.go
# create server keys
SERVER_PRIVATE_KEY=$(wg genkey)
SERVER_PUBLIC_KEY=$(echo $SERVER_PRIVATE_KEY | wg pubkey)
# create new dwgd0 wireguard interface
sudo ip link add dwgd0 type wireguard
echo $SERVER_PRIVATE_KEY | sudo wg set dwgd0 private-key /dev/fd/0 listen-port 51820
sudo ip address add 10.0.0.1/24 dev dwgd0
# bring interface up
sudo ip link set up dev dwgd0
# generate your container's public key with a specific seed
CLIENT_PUBLIC_KEY=$(./dwgd pubkey -i 10.0.0.2 -s supersecretseed)
sudo wg set dwgd0 peer $CLIENT_PUBLIC_KEY allowed-ips 10.0.0.2/32
# run dwgd driver
sudo ./dwgd -v &
# create docker network with the previously set server public key and seed
docker network create --driver=dwgd -o dwgd.endpoint=localhost:51820 -o dwgd.seed=supersecretseed -o dwgd.pubkey=$SERVER_PUBLIC_KEY --subnet="10.0.0.0/24" --gateway=10.0.0.1 dwgd-net
# run your container
docker run -it --rm --network=dwgd-net --ip=10.0.0.2 busybox
```