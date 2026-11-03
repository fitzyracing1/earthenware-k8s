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
	"k8s.io/kubernetes/pkg/scheduler/framework"
	frameworkruntime "k8s.io/kubernetes/pkg/scheduler/framework/runtime"
)

const (
	Name          = "Earthenware"
	TempAnnotation = "earthenware.io/temp-c"
)

// Args configures the plugin. MaxTempC <= 0 disables the heat filter.
type Args struct {
	MaxTempC float64 `json:"maxTempC,omitempty"`
}

type Plugin struct {
	handle framework.Handle
	args   Args
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
	return &Plugin{handle: h, args: args}, nil
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
	pc, pm := podRequests(pod)
	return PackScore(ni.Requested.MilliCPU, ni.Requested.Memory, pc, pm,
		ni.Allocatable.MilliCPU, ni.Allocatable.Memory), nil
}

func (p *Plugin) ScoreExtensions() framework.ScoreExtensions { return nil }
