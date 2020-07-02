#!/usr/bin/env bash

die() {
	code=$?
	echo "$@"
	exit $code
}

if [ ! -f ./storage ]; then
	echo "Storage binary not present. Running make..."
	(cd ..; make) || die "Build failed."
fi

echo "Starting 4 servers..."
addrs=("localhost:10000" "localhost:10001" "localhost:10002" "localhost:10003")

./storage --server "${addrs[0]}" &
./storage --server "${addrs[1]}" &
./storage --server "${addrs[2]}" &
./storage --server "${addrs[3]}" &

./storage --connect "$(echo "${addrs[@]//\ /,}" | sed 's/ /,/g')"

kill $(jobs -p)
