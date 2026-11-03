// Package earthenware implements the Earthenware scheduler plugin:
// Score bin-packs pods onto the fewest nodes; Filter optionally skips nodes
// whose MEASURED temperature (annotation earthenware.io/temp-c) exceeds a threshold.
package earthenware

import (
	"context"
	"fmt"
	"strconv"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"

	"github.com/fitzyracing1/earthenware-k8s/pkg/tree"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	frameworkruntime "k8s.io/kubernetes/pkg/scheduler/framework/runtime"
)

const (
	Name           = "Earthenware"
	TempAnnotation = "earthenware.io/temp-c"
)

// Args configures the plugin. MaxTempC <= 0 disables the heat filter.
type Args struct {
	MaxTempC float64 `json:"maxTempC,omitempty"`
	// TreePath: optional tree.json. When set, pods fill the Earthenware ternary tree
	// slot by slot: VM groups (node label earthenware.io/vm) in tree order, one pod per block.
	TreePath string `json:"treePath,omitempty"`
}

type Plugin struct {
	handle framework.Handle
	args   Args
	tree   *tree.Tree
}

var _ framework.FilterPlugin = &Plugin{}
var _ framework.ScorePlugin = &Plugin{}

func New(_ context.Context, obj runtime.Object, h framework.Handle) (framework.Plugin, error) {
	args := Args{}
	if obj != nil {
		if err := frameworkruntime.DecodeInto(obj, &args); err != nil {
			return nil, err
		}
	}
	p := &Plugin{handle: h, args: args}
	if args.TreePath != "" {
		t, err := tree.Load(args.TreePath)
		if err != nil {
			return nil, fmt.Errorf("earthenware tree: %w", err)
		}
		p.tree = t
	}
	return p, nil
}

func (p *Plugin) Name() string { return Name }

// HeatDecision returns (reject, reason). Missing/unparseable annotation => never reject (no guessing).
func HeatDecision(node *v1.Node, maxTempC float64) (bool, string) {
	if maxTempC <= 0 || node == nil {
		return false, ""
	}
	raw, ok := node.Annotations[TempAnnotation]
	if !ok {
		return false, "no measured temperature annotation; heat filter skipped"
	}
	t, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return false, fmt.Sprintf("unparseable temperature %q; heat filter skipped", raw)
	}
	if t > maxTempC {
		return true, fmt.Sprintf("measured temp %.1fC > max %.1fC", t, maxTempC)
	}
	return false, ""
}

func (p *Plugin) Filter(_ context.Context, _ *framework.CycleState, _ *v1.Pod, ni *framework.NodeInfo) *framework.Status {
	node := ni.Node()
	if p.tree != nil {
		if ok, why := TreeFits(p.tree, node, countEarthenwarePods(ni)); !ok {
			return framework.NewStatus(framework.Unschedulable, why)
		}
	}
	reject, reason := HeatDecision(node, p.args.MaxTempC)
	if reject {
		return framework.NewStatus(framework.Unschedulable, reason)
	}
	if reason != "" {
		klog.V(2).InfoS("Earthenware", "node", node.Name, "msg", reason)
	}
	return nil
}

// PackScore: higher score for nodes with higher requested CPU+memory fraction
// (after placing the pod). Nodes that can't fit get 0 (Fit filter handles rejection).
func PackScore(reqCPU, reqMem, podCPU, podMem, allocCPU, allocMem int64) int64 {
	if allocCPU <= 0 || allocMem <= 0 {
		return 0
	}
	c, m := reqCPU+podCPU, reqMem+podMem
	if c > allocCPU || m > allocMem {
		return 0
	}
	frac := (float64(c)/float64(allocCPU) + float64(m)/float64(allocMem)) / 2
	return int64(frac * float64(framework.MaxNodeScore))
}

func podRequests(pod *v1.Pod) (cpu, mem int64) {
	for _, c := range pod.Spec.Containers {
		cpu += c.Resources.Requests.Cpu().MilliValue()
		mem += c.Resources.Requests.Memory().Value()
	}
	return
}

func (p *Plugin) Score(_ context.Context, _ *framework.CycleState, pod *v1.Pod, nodeName string) (int64, *framework.Status) {
	ni, err := p.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
	if err != nil {
		return 0, framework.AsStatus(err)
	}
	if p.tree != nil {
		return TreeScore(p.tree, ni.Node(), countEarthenwarePods(ni)), nil
	}
	pc, pm := podRequests(pod)
	return PackScore(ni.Requested.MilliCPU, ni.Requested.Memory, pc, pm,
		ni.Allocatable.MilliCPU, ni.Allocatable.Memory), nil
}

func (p *Plugin) ScoreExtensions() framework.ScoreExtensions { return nil }

// SchedulerName is the profile name whose pods occupy tree slots.
const SchedulerName = "earthenware"

func countEarthenwarePods(ni *framework.NodeInfo) int {
	n := 0
	for _, pi := range ni.Pods {
		if pi.Pod.Spec.SchedulerName == SchedulerName {
			n++
		}
	}
	return n
}

// TreeFits: node must belong to a VM group (host/control plane gets no pods)
// and the group must have a free block slot.
func TreeFits(t *tree.Tree, node *v1.Node, used int) (bool, string) {
	g := node.Labels[tree.NodeLabel]
	if t.GroupRank(g) < 0 {
		return false, fmt.Sprintf("node not in an Earthenware VM group (label %s=%q)", tree.NodeLabel, g)
	}
	if used >= len(t.Slots(g)) {
		return false, fmt.Sprintf("group %s: all %d block slots full", g, len(t.Slots(g)))
	}
	return true, ""
}

// TreeScore: fill ternary order. Earlier groups score higher; within the score band,
// fuller groups score higher (bin-pack). Result in [0, MaxNodeScore].
func TreeScore(t *tree.Tree, node *v1.Node, used int) int64 {
	g := node.Labels[tree.NodeLabel]
	r := t.GroupRank(g)
	slots := len(t.Slots(g))
	n := len(t.Groups())
	if r < 0 || slots == 0 || used >= slots {
		return 0
	}
	band := framework.MaxNodeScore / int64(n)
	return int64(n-1-r)*band + int64(used)*(band-1)/int64(slots)
}
