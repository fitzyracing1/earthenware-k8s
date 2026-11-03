# earthenware-k8s

The [Earthenware Computer](https://github.com/fitzyracing1/earthenware-tree) idea (pack small jobs into isolated slots on as few hosts as possible) as a **Kubernetes scheduler plugin**, with [heatgate](https://github.com/fitzyracing1/heatgate)'s principle: act on **measured** data only, never estimates.

## What it does
- **Score (bin-packing):** nodes that are already busier (requested CPU + memory fraction, after placing the pod) score higher, so pods fill the fewest nodes and idle nodes can be scaled away / powered down.
- **Filter (optional heat gate):** if `maxTempC > 0`, nodes whose annotation `earthenware.io/temp-c` is above the threshold are Unschedulable. If the annotation is missing or unparseable, the node is **not** filtered (no guessing) and this is logged at `--v=2`.
- **Agent (`agent/`):** DaemonSet writing `earthenware.io/temp-c` (max of `/sys/class/thermal/*/temp`, or heatgate if installed) and `earthenware.io/cpu-pct` (from `/proc/stat`). With no sensor (typical VMs) it writes no temperature, removes any stale one, and logs that.

Built out-of-tree like [kubernetes-sigs/scheduler-plugins](https://github.com/kubernetes-sigs/scheduler-plugins): `cmd/scheduler` wraps upstream `kube-scheduler` with the extra plugin. Pinned to Kubernetes **v1.31.4** modules.

## Earthenware tree mapping
The cluster layout follows [`tree.json`](deploy/tree.json) from earthenware-tree (copied verbatim): **5 beginnings, 9 steps, 15 blocks**, ternary (every parent ≤ 3 children).

| tree.json | Kubernetes |
|---|---|
| host subtree `C` (1 beginning → 2 steps → 4 blocks, `kind: computer`) | control plane + Earthenware scheduler; gets **no** workload pods |
| each VM beginning `VM1..VM4` (linked only to `C` via `links`) | a node group: nodes labelled `earthenware.io/vm=VMn` |
| each block under a VM (`VM1.s1.b1` …) | one pod slot (VM1=4, VM2=4, VM3=2, VM4=1 → 11 slots) |

With `treePath` set in the plugin args, the scheduler fills the tree slot by slot: VM1's blocks in order, then VM2, VM3, VM4. Filter rejects nodes outside a VM group and full groups; Score ranks earlier groups first (and fuller within a band). Only pods with `schedulerName: earthenware` occupy slots. `pkg/tree` validates the file (one host, ternary, VMs link only to host) and `cmd/earthenware-layout` emits a tree.json-compatible file (all original keys kept, plus a `"pod"` field on each occupied block) from `kubectl get nodes/pods -o json`. Tests use the real tree.json and check counts, fill order, overflow, host exclusion and round-trip. Without `treePath` the plugin falls back to plain request-based bin-packing.

Assumption: one Kubernetes node per VM group (as in `bench/kind-config.yaml`); with several nodes per group, the slot cap currently applies per node, not per group.

## Install
```sh
docker build -t ghcr.io/fitzyracing1/earthenware-scheduler:dev .   # push or kind-load it
kubectl -n kube-system create configmap earthenware-tree --from-file=deploy/tree.json
kubectl apply -f deploy/scheduler.yaml
kubectl apply -f deploy/agent.yaml
kubectl create configmap earthenware-agent -n kube-system --from-file=agent/agent.py --dry-run=client -o yaml | kubectl apply -f -
```
Then set `schedulerName: earthenware` on pods. Config lives in the `earthenware-scheduler-config` ConfigMap (`maxTempC`, default 85 there).

## Benchmark
`bench/run.sh` creates a 5-node kind cluster laid out as the tree (control plane = host `C`, 4 workers = VM1..VM4), runs 11 pods (one per slot), schedules the same pod set with `default-scheduler` and `earthenware`, and records measured: nodes used, pod creation→PodScheduled seconds, pods per node, and `docker stats` CPU% per node. No dollar/watt figures are derived.

**Status: the benchmark has NOT been run yet.** The machine this was built on has no Docker, so there are no results. Run `bench/run.sh` on a host with Docker + kind + kubectl; results are saved to `bench/results-*.txt`.

## Honest status
- Compiles and unit tests pass (scoring and heat filter logic). CI: vet, build, test, agent syntax check, kubeconform on manifests.
- Not yet tested on a real cluster. No container image is published yet.
- Score uses scheduler *requests*, not live usage; live CPU from the agent is recorded but not yet used for scoring.

## Path to upstream
1. Publish standalone here; run the benchmark on kind and on real hardware with sensors; publish the raw measured results.
2. Add integration tests (scheduler-plugins style), plugin args types/validation, and a tagged image.
3. Propose to [kubernetes-sigs/scheduler-plugins](https://github.com/kubernetes-sigs/scheduler-plugins) via an issue/KEP-style proposal with the benchmark data. (Note: upstream already has `NodeResourcesFit` `MostAllocated` scoring; the measured-temperature filter is the main new idea to argue for.)

## License
MIT
