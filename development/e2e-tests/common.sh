#!/bin/bash

# from: https://stackoverflow.com/questions/5947742/how-to-change-the-output-color-of-echo-in-linux
RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m' # No Color

# from: https://tldp.org/LDP/abs/html/debugging.html#ASSERT
assert() {
    E_PARAM_ERR=98
    E_ASSERT_FAILED=99

    if [ -z "$2" ]; then
        return $E_PARAM_ERR
    fi

    assertion=$1
    error_message=$2

    if [ ! $assertion ]; then
        echo -e "${RED}KO${NC} $error_message"
        exit $E_ASSERT_FAILED
    else
        echo -e "${GREEN}OK${NC}"
        return
    fi
}

# from: https://cedwards.xyz/defer-for-shell/
DEFER=
defer() {
    DEFER="$*; ${DEFER}"
    trap "{ $DEFER }" EXIT
}

# from: https://stackoverflow.com/questions/59895/how-do-i-get-the-directory-where-a-bash-script-is-located-from-within-the-script
SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)

REPO_DIR=${SCRIPT_DIR}/../..
DWGD=${REPO_DIR}/dwgd
TEST_DIR=/tmp/dwgd/run
RESULT_DIR=/tmp/dwgd/results
START_DATE=$(date +%+4Y%m%d_%H%M%S)

#*******
# dwgd specific definitions
#*******
DWGD_DB_FILE=${TEST_DIR}/dwgd.db
DWGD_PID_FILE=${TEST_DIR}/dwgd.pid
DWGD_STDOUT_FILE=${TEST_DIR}/dwgd.stdout
DWGD_STDERR_FILE=${TEST_DIR}/dwgd.stderr

# from: https://stackoverflow.com/questions/692000/how-do-i-write-standard-error-to-a-file-while-using-tee-with-a-pipe
dup_stds_to_test_env() {
    exec 1> >(tee $TEST_DIR/stdout.out) 2> >(tee $TEST_DIR/stderr.out >&2)
}

setup_test_env() {
    rm -rf $TEST_DIR
    mkdir -p $TEST_DIR
    dup_stds_to_test_env
    systemctl stop dwgd
    $DWGD -v -d $DWGD_DB_FILE >$DWGD_STDOUT_FILE 2>$DWGD_STDERR_FILE &
    pid=$!
    echo $pid >$DWGD_PID_FILE
}

teardown_test_env() {
    kill $(cat ${DWGD_PID_FILE})
    mkdir -p $RESULT_DIR/run-$START_DATE
    cp -r $TEST_DIR/* $RESULT_DIR/run-$START_DATE
    rm -r $TEST_DIR
}

#*******
# network to which the container will connect to
#*******
NETWORK_IFNAME=dwgd0
NETWORK_PRIVATE_KEY=$(wg genkey)
NETWORK_PUBLIC_KEY=$(echo $NETWORK_PRIVATE_KEY | wg pubkey)
NETWORK_LISTEN_PORT=51820
NETWORK_IP="10.0.0.1"
NETWORK_CIDR="24"
NETWORK_SEED="supersecretseed"

create_wireguard_interface() {
    ip link add name $NETWORK_IFNAME type wireguard
    echo $NETWORK_PRIVATE_KEY | wg set $NETWORK_IFNAME \
        private-key /dev/fd/0 \
        listen-port $NETWORK_LISTEN_PORT
    ip address add $NETWORK_IP/$NETWORK_CIDR dev $NETWORK_IFNAME
    ip link set up dev $NETWORK_IFNAME
}

remove_wireguard_interface() {
    ip link del dev $NETWORK_IFNAME
}

#*******
# client and docker definitions
#*******
CLIENT_IP="10.0.0.2"
CLIENT_PUBLIC_KEY=$(${DWGD} pubkey -i ${CLIENT_IP} -s ${NETWORK_SEED})
DOCKER_NETWORK_NAME="dwgd-net"
