#! /bin/bash

set -e

go build ..

# payload 16

./benchclient -f=1 -mode byzq -p=16 -wr=0
./benchclient -f=2 -mode byzq -p=16 -wr=0
./benchclient -f=3 -mode byzq -p=16 -wr=0
./benchclient -f=4 -mode byzq -p=16 -wr=0
./benchclient -f=5 -mode byzq -p=16 -wr=0

./benchclient -f=1 -mode byzq -p=16 -wr=5
./benchclient -f=2 -mode byzq -p=16 -wr=5
./benchclient -f=3 -mode byzq -p=16 -wr=5
./benchclient -f=4 -mode byzq -p=16 -wr=5
./benchclient -f=5 -mode byzq -p=16 -wr=5

./benchclient -f=1 -mode byzq -p=16 -wr=10
./benchclient -f=2 -mode byzq -p=16 -wr=10
./benchclient -f=3 -mode byzq -p=16 -wr=10
./benchclient -f=4 -mode byzq -p=16 -wr=10
./benchclient -f=5 -mode byzq -p=16 -wr=10

./benchclient -f=1 -mode byzq -p=16 -wr=50
./benchclient -f=2 -mode byzq -p=16 -wr=50
./benchclient -f=3 -mode byzq -p=16 -wr=50
./benchclient -f=4 -mode byzq -p=16 -wr=50
./benchclient -f=5 -mode byzq -p=16 -wr=50

./benchclient -f=1 -mode byzq -p=16 -wr=80
./benchclient -f=2 -mode byzq -p=16 -wr=80
./benchclient -f=3 -mode byzq -p=16 -wr=80
./benchclient -f=4 -mode byzq -p=16 -wr=80
./benchclient -f=5 -mode byzq -p=16 -wr=80

./benchclient -f=1 -mode byzq -p=16 -wr=100
./benchclient -f=2 -mode byzq -p=16 -wr=100
./benchclient -f=3 -mode byzq -p=16 -wr=100
./benchclient -f=4 -mode byzq -p=16 -wr=100
./benchclient -f=5 -mode byzq -p=16 -wr=100

# payload 1024

./benchclient -f=1 -mode byzq -p=1024 -wr=0
./benchclient -f=2 -mode byzq -p=1024 -wr=0
./benchclient -f=3 -mode byzq -p=1024 -wr=0
./benchclient -f=4 -mode byzq -p=1024 -wr=0
./benchclient -f=5 -mode byzq -p=1024 -wr=0

./benchclient -f=1 -mode byzq -p=1024 -wr=5
./benchclient -f=2 -mode byzq -p=1024 -wr=5
./benchclient -f=3 -mode byzq -p=1024 -wr=5
./benchclient -f=4 -mode byzq -p=1024 -wr=5
./benchclient -f=5 -mode byzq -p=1024 -wr=5

./benchclient -f=1 -mode byzq -p=1024 -wr=10
./benchclient -f=2 -mode byzq -p=1024 -wr=10
./benchclient -f=3 -mode byzq -p=1024 -wr=10
./benchclient -f=4 -mode byzq -p=1024 -wr=10
./benchclient -f=5 -mode byzq -p=1024 -wr=10

./benchclient -f=1 -mode byzq -p=1024 -wr=50
./benchclient -f=2 -mode byzq -p=1024 -wr=50
./benchclient -f=3 -mode byzq -p=1024 -wr=50
./benchclient -f=4 -mode byzq -p=1024 -wr=50
./benchclient -f=5 -mode byzq -p=1024 -wr=50

./benchclient -f=1 -mode byzq -p=1024 -wr=80
./benchclient -f=2 -mode byzq -p=1024 -wr=80
./benchclient -f=3 -mode byzq -p=1024 -wr=80
./benchclient -f=4 -mode byzq -p=1024 -wr=80
./benchclient -f=5 -mode byzq -p=1024 -wr=80

./benchclient -f=1 -mode byzq -p=1024 -wr=100
./benchclient -f=2 -mode byzq -p=1024 -wr=100
./benchclient -f=3 -mode byzq -p=1024 -wr=100
./benchclient -f=4 -mode byzq -p=1024 -wr=100
./benchclient -f=5 -mode byzq -p=1024 -wr=100
