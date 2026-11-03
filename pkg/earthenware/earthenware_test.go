package earthenware

import (
	"testing"

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
