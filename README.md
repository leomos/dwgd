# dwgd: Docker WireGuard Driver

**dwgd** is a Docker plugin that let your containers connect to a WireGuard server.
This is achieved by [moving a WireGuard network interface](https://www.wireguard.com/netns/) from `dwgd` running namespace into the designated container namespace.

**Credits**: this is a rewrite of the proof of concept presented in [this great article](https://www.bestov.io/blog/using-wireguard-as-the-network-for-a-docker-container).

## Example

Generate the public key given your seed and the IP address that your container will have:
```
$ dwgd pubkey -s supersecretseed -i 10.0.0.2
oKetpvdq/I/c7hTW6/AtQPqVlSzgx3q2ClWCx/OXS00=
```

Start dwgd:
```
$ sudo dwgd -d /var/lib/dwgd.db
[...]
```

Create the docker network with the same seed you used above:
```
$ docker network create \
    --driver=dwgd \
    -o dwgd.endpoint=example.com:51820 \
    -o dwgd.seed=supersecretseed \
    -o dwgd.pubkey="your server's public key" \
    --subnet=10.0.0.0/24 \
    --gateway=10.0.0.1 \
    dwgd_net
```

Start a docker container with the network you just created:
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

So far it has been tested in a Linux machine with Ubuntu 20.04, but I guess it could work on any reasonably recent Linux system that respects the dependencies.

After cloning the repository you can build the binary and optionally install the systemd units.
```
$ go build -o /usr/bin/dwgd ./cmd/dwgd.go
$ chmod +x /usr/bin/dwgd
$ install init/* /etc/systemd/system/
```

### Dependencies

You need to have WireGuard installed on your system and the `iproute2` package: `dwgd` uses the `ip` command to create and delete the WireGuard interfaces.

## Limitations

Currently `dwgd` sets the WireGuard interface as the default route, in the future this issue will be addressed.