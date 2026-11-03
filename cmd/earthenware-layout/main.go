// earthenware-layout emits a tree.json-compatible layout of the live cluster:
//
//	kubectl get nodes -o json > nodes.json
//	kubectl get pods -A -o json > pods.json
//	earthenware-layout -tree tree.json -nodes nodes.json -pods pods.json > layout.json
//
// Each block gets a "pod" field (namespace/name) for pods scheduled by "earthenware",
// in tree order within the node's VM group.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"

	v1 "k8s.io/api/core/v1"

	"github.com/fitzyracing1/earthenware-k8s/pkg/tree"
)

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func main() {
	tp := flag.String("tree", "tree.json", "tree.json path")
	np := flag.String("nodes", "nodes.json", "kubectl get nodes -o json")
	pp := flag.String("pods", "pods.json", "kubectl get pods -A -o json")
	flag.Parse()
	t, err := tree.Load(*tp)
	must(err)
	var nodes v1.NodeList
	var pods v1.PodList
	b, err := os.ReadFile(*np)
	must(err)
	must(json.Unmarshal(b, &nodes))
	b, err = os.ReadFile(*pp)
	must(err)
	must(json.Unmarshal(b, &pods))
	group := map[string]string{}
	for _, n := range nodes.Items {
		group[n.Name] = n.Labels[tree.NodeLabel]
	}
	sort.Slice(pods.Items, func(i, j int) bool {
		return pods.Items[i].CreationTimestamp.Before(&pods.Items[j].CreationTimestamp)
	})
	by := map[string][]string{}
	for _, p := range pods.Items {
		if p.Spec.SchedulerName == "earthenware" && p.Spec.NodeName != "" {
			g := group[p.Spec.NodeName]
			by[g] = append(by[g], p.Namespace+"/"+p.Name)
		}
	}
	out, over := t.Fill(by)
	for _, o := range over {
		fmt.Fprintln(os.Stderr, "pod outside tree slots:", o)
	}
	j, err := json.Marshal(out)
	must(err)
	os.Stdout.Write(append(j, '\n'))
}
