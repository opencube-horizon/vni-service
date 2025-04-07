#!/bin/bash

/opt/k3s/k3s-arm64  kubectl -n vnitest delete vnic --all 1>/dev/null 2>&1
