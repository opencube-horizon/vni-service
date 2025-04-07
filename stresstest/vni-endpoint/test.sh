#!/usr/bin/env bash

generate() {
  local lower=$1
  local upper=$2
	local out=""
	for i in $(seq "$lower" "$upper"); do
		read -r -d '' out <<-EOF
$out
---
apiVersion: horizon-opencube.eu/v1
kind: VniClaim
metadata:
  name: vni-claim-test-$i
  namespace: vnitest
spec:
  name: test-$i
EOF
	done
	echo "$out"
}
generate 1 250 > /tmp/g1
generate 251 500 > /tmp/g2
generate 501 750 > /tmp/g3
generate 751 1000 > /tmp/g4

generate 1001 1250 > /tmp/g5
generate 1251 1500 > /tmp/g6
generate 1501 1750 > /tmp/g7
generate 1751 2000 > /tmp/g8

/opt/k3s/k3s-arm64 kubectl apply -f /tmp/g1 1>/dev/null 2>&1 &
/opt/k3s/k3s-arm64 kubectl apply -f /tmp/g2 1>/dev/null 2>&1 &
/opt/k3s/k3s-arm64 kubectl apply -f /tmp/g3 1>/dev/null 2>&1 &
/opt/k3s/k3s-arm64 kubectl apply -f /tmp/g4 1>/dev/null 2>&1 &

/opt/k3s/k3s-arm64 kubectl apply -f /tmp/g5 1>/dev/null 2>&1 &
/opt/k3s/k3s-arm64 kubectl apply -f /tmp/g6 1>/dev/null 2>&1 &
/opt/k3s/k3s-arm64 kubectl apply -f /tmp/g7 1>/dev/null 2>&1 &
/opt/k3s/k3s-arm64 kubectl apply -f /tmp/g8 1>/dev/null 2>&1 &