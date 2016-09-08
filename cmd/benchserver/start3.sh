#! /bin/bash

set -e

go build

./benchserver -port=8080 &
./benchserver -port=8081 &
./benchserver -port=8082 &

echo "running, enter to stop"

read && killall benchserver 
