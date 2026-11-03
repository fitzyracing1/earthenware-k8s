package earthenware

import (
	"testing"

	"github.com/fitzyracing1/earthenware-k8s/pkg/tree"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPackScorePrefersBusy(t *testing.T) {
	idle := PackScore(0, 0, 500, 1<<28, 4000, 8<<30)
	busy := PackScore(2000, 4<<30, 500, 1<<28, 4000, 8<<30)
	if busy <= idle {
		t.Fatalf("busy %d should beat idle %d", busy, idle)
	}
	if PackScore(3900, 0, 500, 0, 4000, 8<<30) != 0 {
		t.Fatal("overfull node should score 0")
	}
	if PackScore(0, 0, 0, 0, 0, 0) != 0 {
		t.Fatal("zero allocatable should score 0")
	}
}

func node(ann map[string]string) *v1.Node {
	return &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n", Annotations: ann}}
}

func TestHeatDecision(t *testing.T) {
	cases := []struct {
		ann    map[string]string
		max    float64
		reject bool
		logged bool
	}{
		{map[string]string{TempAnnotation: "85"}, 80, true, true},
		{map[string]string{TempAnnotation: "60"}, 80, false, false},
		{nil, 80, false, true},
		{map[string]string{TempAnnotation: "abc"}, 80, false, true},
		{map[string]string{TempAnnotation: "99"}, 0, false, false},
	}
	for i, c := range cases {
		r, why := HeatDecision(node(c.ann), c.max)
		if r != c.reject || (why != "") != c.logged {
			t.Errorf("case %d: got %v %q", i, r, why)
		}
	}
}

func vmNode(g string) *v1.Node {
	return &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: g, Labels: map[string]string{tree.NodeLabel: g}}}
}

func TestTreeFillOrder(t *testing.T) {
	tr, err := tree.Load("../tree/testdata/tree.json")
	if err != nil {
		t.Fatal(err)
	}
	// Simulate scheduling 11 pods: each time pick the highest-scoring fitting node.
	used := map[string]int{}
	var order []string
	for i := 0; i < 11; i++ {
		best, bs := "", int64(-1)
		for _, g := range []string{"VM4", "VM3", "VM2", "VM1"} {
			if ok, _ := TreeFits(tr, vmNode(g), used[g]); ok {
				if s := TreeScore(tr, vmNode(g), used[g]); s > bs {
					best, bs = g, s
				}
			}
		}
		used[best]++
		order = append(order, best)
	}
	want := []string{"VM1", "VM1", "VM1", "VM1", "VM2", "VM2", "VM2", "VM2", "VM3", "VM3", "VM4"}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order %v", order)
		}
	}
	if ok, _ := TreeFits(tr, vmNode("VM4"), 1); ok {
		t.Fatal("VM4 has 1 slot; should be full")
	}
	host := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "cp"}}
	if ok, _ := TreeFits(tr, host, 0); ok {
		t.Fatal("host/control plane must not take pods")
	}
}
