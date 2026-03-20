#!/bin/bash
# Create fake /dev/nvidia* device nodes for CI testing.
# Docker mounts /dev at runtime, so these must be created at runtime, not build time.
for i in $(seq 0 7); do
    [ -e /dev/nvidia$i ] || touch /dev/nvidia$i
done
[ -e /dev/nvidiactl ] || touch /dev/nvidiactl
[ -e /dev/nvidia-uvm ] || touch /dev/nvidia-uvm
[ -e /dev/nvidia-uvm-tools ] || touch /dev/nvidia-uvm-tools
