#!/bin/bash

echo "QuorumCall with Multicast"
go test -bench=BenchmarkQCMulticast -benchmem -count=10 -run=^# -benchtime=$1x > qcm.profile
echo "QuorumCall with BroadcastOption"
go test -bench=BenchmarkQCBroadcastOption -benchmem -count=10 -run=^# -benchtime=$1x > qcb.profile
echo "BroadcastCall"
go test -bench=BenchmarkBroadcastCall -benchmem -count=10 -run=^# -benchtime=$1x > bc.profile