#!/usr/bin/env bash
set -euo pipefail
source /mnt/c/Projects/velora/scripts/env.sh

echo "=== Images needed by failing pods ==="
kubectl get pods -A -o json | python3 -c '
import json, sys
data = json.load(sys.stdin)
images = set()
for pod in data["items"]:
    phase = pod["status"].get("phase","")
    if phase in ("Running","Succeeded"):
        continue
    for cs in pod.get("spec",{}).get("containers",[]):
        images.add(cs.get("image",""))
    for cs in pod.get("spec",{}).get("initContainers",[]):
        images.add(cs.get("image",""))
for img in sorted(images):
    print(img)
'

echo ""
echo "=== Images already cached on Docker host ==="
docker images --format '{{.Repository}}:{{.Tag}}' | sort
