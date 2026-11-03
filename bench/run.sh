#!/usr/bin/env bash
# Reproducible benchmark: default scheduler vs Earthenware on a 4-worker kind cluster.
# Prints MEASURED numbers only (nodes used, pod pending->scheduled seconds, docker stats CPU per node).
set -euo pipefail
cd "$(dirname "$0")/.."
N=${PODS:-11}  # = VM block slots in tree.json
CLUSTER=ek8s-bench
OUT=bench/results-$(date +%Y%m%d-%H%M%S).txt
command -v docker && command -v kind && command -v kubectl >/dev/null || { echo "need docker, kind, kubectl"; exit 1; }
kind create cluster --name $CLUSTER --config bench/kind-config.yaml
trap 'kind delete cluster --name $CLUSTER' EXIT
docker build -t ghcr.io/fitzyracing1/earthenware-scheduler:dev .
kind load docker-image ghcr.io/fitzyracing1/earthenware-scheduler:dev --name $CLUSTER
kubectl -n kube-system create configmap earthenware-tree --from-file=deploy/tree.json
sed 's/image: ghcr.io/imagePullPolicy: IfNotPresent\n        image: ghcr.io/' deploy/scheduler.yaml | kubectl apply -f -
kubectl -n kube-system rollout status deploy/earthenware-scheduler --timeout=180s

run() { # $1 = schedulerName
  ns=bench-$1; kubectl create ns $ns
  for i in $(seq 1 $N); do cat <<Y; echo ---; done | kubectl apply -n $ns -f - >/dev/null
apiVersion: v1
kind: Pod
metadata: {name: w$i}
spec:
  schedulerName: $1
  containers: [{name: c, image: registry.k8s.io/pause:3.9, resources: {requests: {cpu: 100m, memory: 64Mi}}}]
Y
  kubectl wait -n $ns --for=condition=PodScheduled pod --all --timeout=300s >/dev/null
  echo "== scheduler=$1 pods=$N"
  echo "nodes_used: $(kubectl get pod -n $ns -o jsonpath='{range .items[*]}{.spec.nodeName}{"\n"}{end}' | sort -u | wc -l)"
  kubectl get pod -n $ns -o json | python3 -c '
import json,sys,datetime as d
p=lambda s:d.datetime.strptime(s,"%Y-%m-%dT%H:%M:%SZ")
w=[(p([c for c in i["status"]["conditions"] if c["type"]=="PodScheduled"][0]["lastTransitionTime"])-p(i["metadata"]["creationTimestamp"])).total_seconds() for i in json.load(sys.stdin)["items"]]
w.sort();print(f"pending_s: max={w[-1]} median={w[len(w)//2]} (1s timestamp resolution)")'
  kubectl get pod -n $ns -o jsonpath='{range .items[*]}{.spec.nodeName}{"\n"}{end}' | sort | uniq -c
  echo "docker stats (CPU% per kind node, one sample):"; docker stats --no-stream --format '{{.Name}} {{.CPUPerc}}' | grep $CLUSTER
  if [ "$1" = earthenware ]; then
    kubectl get nodes -o json > /tmp/n.json; kubectl get pods -A -o json > /tmp/p.json
    go run ./cmd/earthenware-layout -tree deploy/tree.json -nodes /tmp/n.json -pods /tmp/p.json > "${OUT%.txt}-layout.json"
    echo "tree layout: ${OUT%.txt}-layout.json"
  fi
  kubectl delete ns $ns --wait >/dev/null
}
{ run default-scheduler; run earthenware; } | tee "$OUT"
echo "saved $OUT"
