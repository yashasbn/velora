#!/usr/bin/env bash
set -euo pipefail

MTU=1280
MSS=1220

echo "=== Tuning MTU to $MTU across entire kind network ==="

# 1. Find the kind bridge on the host
KIND_BRIDGE=$(docker network inspect kind -f '{{.Id}}' | cut -c1-12)
BRIDGE_IF="br-${KIND_BRIDGE}"
echo "Kind bridge: $BRIDGE_IF"

# 2. Set MTU on the host bridge
echo "[host] Setting $BRIDGE_IF MTU=$MTU ..."
sudo ip link set dev "$BRIDGE_IF" mtu "$MTU"

# 3. Set MTU on all host-side veth peers attached to the kind bridge
for veth in $(bridge link show | grep "$BRIDGE_IF" | awk '{print $2}' | tr -d ':' | cut -d@ -f1); do
  echo "[host] Setting $veth MTU=$MTU ..."
  sudo ip link set dev "$veth" mtu "$MTU" 2>/dev/null || true
done

# 4. Set MTU on the WSL eth0 (host egress)
echo "[host] Setting eth0 MTU=$MTU ..."
sudo ip link set dev eth0 mtu "$MTU" 2>/dev/null || true

# 5. Add MSS clamping on the host
echo "[host] Adding iptables MSS clamp ..."
sudo iptables -t mangle -F POSTROUTING 2>/dev/null || true
sudo iptables -t mangle -A POSTROUTING -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --set-mss "$MSS"

# 6. Set MTU on all interfaces inside each kind node
for node in velora-control-plane velora-worker velora-worker2; do
  echo "[$node] Setting all interfaces MTU=$MTU ..."
  docker exec "$node" sh -c "
    for iface in \$(ip -o link show | awk -F: '{print \$2}' | tr -d ' ' | grep -v lo); do
      ip link set dev \$iface mtu $MTU 2>/dev/null || true
    done
    iptables -t mangle -F POSTROUTING 2>/dev/null || true
    iptables -t mangle -A POSTROUTING -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --set-mss $MSS
    echo 'OK'
  "
done

echo ""
echo "=== Verifying connectivity ==="
echo -n "quay.io TLS: "
docker exec velora-worker curl -s --connect-timeout 15 -o /dev/null -w "%{http_code}" https://quay.io/v2/ 2>&1 || echo "FAILED"
echo ""
echo -n "docker.io TLS: "
docker exec velora-worker curl -s --connect-timeout 15 -o /dev/null -w "%{http_code}" https://registry-1.docker.io/v2/ 2>&1 || echo "FAILED"
echo ""
