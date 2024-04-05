# dwgd: Docker WireGuard Driver

**dwgd** is a Docker plugin that let your containers connect to a WireGuard 
network.
This is achieved by [moving a WireGuard network interface](https://www.wireguard.com/netns/)
from `dwgd` running namespace into the designated container namespace.

## Usage

### 1. Start the daemon

Start the `dwgd` daemon:
```
$ sudo dwgd -d /var/lib/dwgd.db
[...]
```

### 2. Create the docker network

Depending on which [driver specific options](https://docs.docker.com/reference/cli/docker/network/create/#options)
(`-o`) you pass during the network creation phase, you can select two modes:

- [ifname mode](#ifname-mode): if you want to connect to a WireGuard
interface that's living in the **same host** as the one you are running your 
containers on;

- [pubkey mode](#pubkey-mode): if you want to connect to a WireGuard interface 
that's living in a **different host** as the one you are running your 
containers on.


#### Ifname mode

In this mode, the name of a **local** WireGuard interface is passed as an option.
`dwgd` will create a new WireGuard interface that will peer to the one you
passed and hand it to the containers.

Options that you need to pass:

- `dwgd.ifname`: the name of the **local** interface;
- `dwgd.seed`: secret seed that will be used to generate public and private keys
by SHA256 hashing the `{IP, seed}` couple.

```
docker network create \
    --driver=dwgd \
    -o dwgd.ifname=wg0 \
    -o dwgd.seed=supersecretseed \
    --subnet=10.0.0.0/24 \
    --gateway=10.0.0.1 \
    dwgd-net
```

#### Pubkey mode

In this mode, an endpoint and a public key for a WireGuard peer to which
containers should connect to are passed as arguments.


**Note**

Please note that you will likely need to modify manually the configuration of
the remote WireGuard peer by adding each container as a peer.

This is doable because public and private keys are deterministically generated
by hashing the `{IP, seed}` couple.

You can generate the public key for an `{IP, seed}` couple using the following
command:

```
$ dwgd pubkey -s supersecretseed -i 10.0.0.2
oKetpvdq/I/c7hTW6/AtQPqVlSzgx3q2ClWCx/OXS00=
```

Options that you need to pass:

- `dwgd.pubkey`: the public key of the remote WireGuard interface;
- `dwgd.seed`: secret seed that will be used to generate public and private keys
by SHA256 hashing the `{IP, seed}` couple;
- `dwgd.endpoint`: the endpoint of the WireGuard peer you want your docker
containers to connect to.

Create the docker network with the same seed you used to generate the public
key:
```
$ docker network create \
    --driver=dwgd \
    -o dwgd.endpoint=example.com:51820 \
    -o dwgd.seed=supersecretseed \
    -o dwgd.pubkey="your remote WireGuard peer's public key" \
    --subnet=10.0.0.0/24 \
    --gateway=10.0.0.1 \
    dwgd_net
```

### 3. Start a container

Note that the IP must be set manually.

```
$ docker run -it --rm --network=dwgd_net --ip=10.0.0.2 busybox
/ # ip a
1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue qlen 1000
    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
    inet 127.0.0.1/8 scope host lo
       valid_lft forever preferred_lft forever
5: wg0: <POINTOPOINT,NOARP,UP,LOWER_UP> mtu 1420 qdisc noqueue qlen 1000
    link/[65534]
    inet 10.0.0.2/24 brd 10.0.0.255 scope global wg0
       valid_lft forever preferred_lft forever
/ # ping 10.0.0.1
PING 10.0.0.1 (10.0.0.1) 56(84) bytes of data.
64 bytes from 10.0.0.1: icmp_seq=1 ttl=54 time=9.98 ms
64 bytes from 10.0.0.1: icmp_seq=2 ttl=54 time=8.65 ms
64 bytes from 10.0.0.1: icmp_seq=3 ttl=54 time=8.34 ms
^C
--- 10.0.0.1 ping statistics ---
3 packets transmitted, 3 received, 0% packet loss, time 2003ms
rtt min/avg/max/mdev = 8.343/8.990/9.976/0.708 ms
```

## Installation

This software has been tested in a Linux machine with Debian 12, but I guess it
could work on any reasonably recent Linux system that respects the dependencies.

After cloning the repository you can build the binary and optionally install
the systemd unit.
```
$ go build -o /usr/bin/dwgd ./cmd/dwgd.go
$ chmod +x /usr/bin/dwgd
$ install systemd/* /etc/systemd/system/
```

### Dependencies

You need to have WireGuard installed on your system and the `iproute2` package:
`dwgd` uses the `ip` command to create and delete the WireGuard interfaces.

You will also need the `nsenter` binary if you want `dwgd` to work with docker
rootless.

## Development

Please refer to [the development directory](development/README.md).

## Credits

This is a rewrite of the proof of concept presented in [this great article](https://www.bestov.io/blog/using-wireguard-as-the-network-for-a-docker-container).