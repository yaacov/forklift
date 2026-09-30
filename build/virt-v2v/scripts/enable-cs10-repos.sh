#!/usr/bin/env bash
# Enable CentOS Stream 10 repos on UBI (virt-v2v, libguestfs, kernel, etc.).
set -euo pipefail

dnf install -y curl
install -d -m 0755 /etc/yum.repos.d
cp -f /tmp/cs10-repos/centos-stream10.repo /etc/yum.repos.d/centos-stream10.repo
