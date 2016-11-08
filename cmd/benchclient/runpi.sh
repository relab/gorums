#! /bin/bash

set -e

# payload 16

../benchclient -mode byzq -p=16 -wr=0 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080"
../benchclient -mode byzq -p=16 -wr=0 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080,pi34:8080,pi35:8080,pi37:8080"
../benchclient -mode byzq -p=16 -wr=0 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080,pi34:8080,pi35:8080,pi37:8080,pi39:8080,pi41:8080,pi26:8080"
../benchclient -mode byzq -p=16 -wr=0 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080,pi34:8080,pi35:8080,pi37:8080,pi39:8080,pi41:8080,pi26:8080,pi27:8080,pi28:8080,pi29:8080"

../benchclient -mode byzq -p=16 -wr=5 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080"
../benchclient -mode byzq -p=16 -wr=5 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080,pi34:8080,pi35:8080,pi37:8080"
../benchclient -mode byzq -p=16 -wr=5 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080,pi34:8080,pi35:8080,pi37:8080,pi39:8080,pi41:8080,pi26:8080"
../benchclient -mode byzq -p=16 -wr=5 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080,pi34:8080,pi35:8080,pi37:8080,pi39:8080,pi41:8080,pi26:8080,pi27:8080,pi28:8080,pi29:8080"

../benchclient -mode byzq -p=16 -wr=10 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080"
../benchclient -mode byzq -p=16 -wr=10 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080,pi34:8080,pi35:8080,pi37:8080"
../benchclient -mode byzq -p=16 -wr=10 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080,pi34:8080,pi35:8080,pi37:8080,pi39:8080,pi41:8080,pi26:8080"
../benchclient -mode byzq -p=16 -wr=10 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080,pi34:8080,pi35:8080,pi37:8080,pi39:8080,pi41:8080,pi26:8080,pi27:8080,pi28:8080,pi29:8080"

../benchclient -mode byzq -p=16 -wr=50 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080"
../benchclient -mode byzq -p=16 -wr=50 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080,pi34:8080,pi35:8080,pi37:8080"
../benchclient -mode byzq -p=16 -wr=50 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080,pi34:8080,pi35:8080,pi37:8080,pi39:8080,pi41:8080,pi26:8080"
../benchclient -mode byzq -p=16 -wr=50 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080,pi34:8080,pi35:8080,pi37:8080,pi39:8080,pi41:8080,pi26:8080,pi27:8080,pi28:8080,pi29:8080"

../benchclient -mode byzq -p=16 -wr=80 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080"
../benchclient -mode byzq -p=16 -wr=80 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080,pi34:8080,pi35:8080,pi37:8080"
../benchclient -mode byzq -p=16 -wr=80 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080,pi34:8080,pi35:8080,pi37:8080,pi39:8080,pi41:8080,pi26:8080"
../benchclient -mode byzq -p=16 -wr=80 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080,pi34:8080,pi35:8080,pi37:8080,pi39:8080,pi41:8080,pi26:8080,pi27:8080,pi28:8080,pi29:8080"

../benchclient -mode byzq -p=16 -wr=100 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080"
../benchclient -mode byzq -p=16 -wr=100 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080,pi34:8080,pi35:8080,pi37:8080"
../benchclient -mode byzq -p=16 -wr=100 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080,pi34:8080,pi35:8080,pi37:8080,pi39:8080,pi41:8080,pi26:8080"
../benchclient -mode byzq -p=16 -wr=100 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080,pi34:8080,pi35:8080,pi37:8080,pi39:8080,pi41:8080,pi26:8080,pi27:8080,pi28:8080,pi29:8080"

# payload 1024

../benchclient -mode byzq -p=1024 -wr=0 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080"
../benchclient -mode byzq -p=1024 -wr=0 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080,pi34:8080,pi35:8080,pi37:8080"
../benchclient -mode byzq -p=1024 -wr=0 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080,pi34:8080,pi35:8080,pi37:8080,pi39:8080,pi41:8080,pi26:8080"
../benchclient -mode byzq -p=1024 -wr=0 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080,pi34:8080,pi35:8080,pi37:8080,pi39:8080,pi41:8080,pi26:8080,pi27:8080,pi28:8080,pi29:8080"

../benchclient -mode byzq -p=1024 -wr=5 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080"
../benchclient -mode byzq -p=1024 -wr=5 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080,pi34:8080,pi35:8080,pi37:8080"
../benchclient -mode byzq -p=1024 -wr=5 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080,pi34:8080,pi35:8080,pi37:8080,pi39:8080,pi41:8080,pi26:8080"
../benchclient -mode byzq -p=1024 -wr=5 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080,pi34:8080,pi35:8080,pi37:8080,pi39:8080,pi41:8080,pi26:8080,pi27:8080,pi28:8080,pi29:8080"

../benchclient -mode byzq -p=1024 -wr=10 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080"
../benchclient -mode byzq -p=1024 -wr=10 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080,pi34:8080,pi35:8080,pi37:8080"
../benchclient -mode byzq -p=1024 -wr=10 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080,pi34:8080,pi35:8080,pi37:8080,pi39:8080,pi41:8080,pi26:8080"
../benchclient -mode byzq -p=1024 -wr=10 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080,pi34:8080,pi35:8080,pi37:8080,pi39:8080,pi41:8080,pi26:8080,pi27:8080,pi28:8080,pi29:8080"

../benchclient -mode byzq -p=1024 -wr=50 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080"
../benchclient -mode byzq -p=1024 -wr=50 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080,pi34:8080,pi35:8080,pi37:8080"
../benchclient -mode byzq -p=1024 -wr=50 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080,pi34:8080,pi35:8080,pi37:8080,pi39:8080,pi41:8080,pi26:8080"
../benchclient -mode byzq -p=1024 -wr=50 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080,pi34:8080,pi35:8080,pi37:8080,pi39:8080,pi41:8080,pi26:8080,pi27:8080,pi28:8080,pi29:8080"

../benchclient -mode byzq -p=1024 -wr=80 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080"
../benchclient -mode byzq -p=1024 -wr=80 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080,pi34:8080,pi35:8080,pi37:8080"
../benchclient -mode byzq -p=1024 -wr=80 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080,pi34:8080,pi35:8080,pi37:8080,pi39:8080,pi41:8080,pi26:8080"
../benchclient -mode byzq -p=1024 -wr=80 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080,pi34:8080,pi35:8080,pi37:8080,pi39:8080,pi41:8080,pi26:8080,pi27:8080,pi28:8080,pi29:8080"

../benchclient -mode byzq -p=1024 -wr=100 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080"
../benchclient -mode byzq -p=1024 -wr=100 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080,pi34:8080,pi35:8080,pi37:8080"
../benchclient -mode byzq -p=1024 -wr=100 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080,pi34:8080,pi35:8080,pi37:8080,pi39:8080,pi41:8080,pi26:8080"
../benchclient -mode byzq -p=1024 -wr=100 -addrs "pi30:8080,pi31:8080,pi32:8080,pi33:8080,pi34:8080,pi35:8080,pi37:8080,pi39:8080,pi41:8080,pi26:8080,pi27:8080,pi28:8080,pi29:8080"
