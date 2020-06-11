#!/usr/bin/env bash
inventory="$1"
args="${*:2}"

out=$(ANSIBLE_STDOUT_CALLBACK=debug ansible-playbook -i "$inventory" -e "bench_args='$args'" benchmark.yml)
status=$?

if [ $status -eq 0 ]; then
	echo "$out" | sed -n '/^MSG:/,/^PLAY/p' | sed '1d;$d'
else
	echo -e "Error:\n$out"
	exit $status
fi
