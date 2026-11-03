package tree

import (
	"encoding/json"
	"testing"
)

func TestRealTreeJSON(t *testing.T) {
	tr, err := Load("testdata/tree.json")
	if err != nil {
		t.Fatal(err)
	}
	b, s, k := tr.Counts()
	if b != 5 || s != 9 || k != 15 {
		t.Fatalf("counts %d/%d/%d want 5/9/15", b, s, k)
	}
	if hb := tr.Slots("C"); len(hb) != 0 {
		// host is kind=computer, not a VM group; Slots still works structurally but Groups excludes it
	}
	g := tr.Groups()
	if len(g) != 4 || g[0] != "VM1" {
		t.Fatalf("groups %v", g)
	}
	total := 0
	for _, x := range g {
		total += len(tr.Slots(x))
	}
	if total != 11 { // 15 blocks - 4 host blocks
		t.Fatalf("vm slots %d want 11", total)
	}
	if s := tr.Slots("VM1"); s[0] != "VM1.s1.b1" || s[1] != "VM1.s1.b2" || s[2] != "VM1.s2.b1" {
		t.Fatalf("fill order %v", s)
	}
}

func TestFillAndEmit(t *testing.T) {
	tr, _ := Load("testdata/tree.json")
	out, over := tr.Fill(map[string][]string{"VM1": {"a", "b", "c", "d", "e"}})
	if len(over) != 1 || over[0] != "e" {
		t.Fatalf("overflow %v", over)
	}
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	back, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := back.Extra["products"]; !ok {
		t.Fatal("products key lost")
	}
	for _, n := range back.Nodes {
		if n.ID == "VM1.s2.b2" && n.Pod != "d" {
			t.Fatalf("got %q", n.Pod)
		}
	}
}

func TestValidateRejects(t *testing.T) {
	bad := `{"links":{"VM1":"X"},"nodes":[{"id":"C","kind":"computer","level":"beginning","parent":null},{"id":"VM1","kind":"vm","level":"beginning","parent":null}]}`
	if _, err := Parse([]byte(bad)); err == nil {
		t.Fatal("want link error")
	}
	four := `{"links":{},"nodes":[{"id":"C","kind":"computer","level":"beginning","parent":null},
{"id":"a","parent":"C"},{"id":"b","parent":"C"},{"id":"c","parent":"C"},{"id":"d","parent":"C"}]}`
	if _, err := Parse([]byte(four)); err == nil {
		t.Fatal("want ternary error")
	}
}
