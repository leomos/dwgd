#!/bin/bash

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)

source ${SCRIPT_DIR}/common.sh

setup_test_env
defer teardown_test_env

create_wireguard_interface
defer remove_wireguard_interface

wg set $NETWORK_IFNAME peer $CLIENT_PUBLIC_KEY allowed-ips $CLIENT_IP/32
assert "$? -eq 0" "Could not create wireguard interface ${NETWORK_IFNAME}"

docker network create \
    --driver=dwgd \
    -o dwgd.pubkey=$NETWORK_PUBLIC_KEY \
    -o dwgd.endpoint=localhost:$NETWORK_LISTEN_PORT \
    -o dwgd.seed=$NETWORK_SEED \
    --subnet="${NETWORK_IP}/${NETWORK_CIDR}" \
    --gateway=$NETWORK_IP \
    dwgd-net
assert "$? -eq 0" "Could not create network ${DOCKER_NETWORK_NAME}"

docker run \
    -it \
    --rm \
    --network=$DOCKER_NETWORK_NAME \
    --ip=$CLIENT_IP \
    busybox \
    ping -c 3 $NETWORK_IP
assert "$? -eq 0" "Could not ping ${NETWORK_IP}"

docker network rm $DOCKER_NETWORK_NAME
assert "$? -eq 0" "Could not remove network ${DOCKER_NETWORK_NAME}"
