#! /bin/bash

set -e

go build

./gqserver -port=8080 -id=0:0 &
./gqserver -port=8081 -id=0:1 &
./gqserver -port=8082 -id=0:1 &

./gqserver -port=8083 -id=1:0 &
./gqserver -port=8084 -id=1:1 &
./gqserver -port=8085 -id=1:1 &

./gqserver -port=8086 -id=2:0 &
./gqserver -port=8087 -id=2:1 &
./gqserver -port=8088 -id=2:1 &

echo "running, enter to stop"

read && killall gqserver 
