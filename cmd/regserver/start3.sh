#! /bin/bash

set -e

go build

./regserver -port=8080 &
./regserver -port=8081 &
./regserver -port=8082 &

echo "running, enter to stop"

read && killall regserver 
