#!/bin/bash

set -e

go build

./storageserver -port=8080 &
./storageserver -port=8081 &
./storageserver -port=8082 &

echo "running, enter to stop"

read && killall storageserver 
