package project

import (
	"path/filepath"
	"testing"
)

func TestRegistryAddListRemove(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry(func() {})
	p, err := reg.Add(dir)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if p.ID == "" || p.Name != filepath.Base(dir) || p.Root != dir {
		t.Fatalf("bad project: %+v", p)
	}
	if got := len(reg.List()); got != 1 {
		t.Fatalf("List = %d, want 1", got)
	}
	if err := reg.Remove(p.ID); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if got := len(reg.List()); got != 0 {
		t.Fatalf("after remove List = %d", got)
	}
	if _, err := reg.Add(dir); err != nil {
		t.Fatalf("re-add: %v", err)
	}
	if _, err := reg.Add(dir); err == nil {
		t.Fatal("adding same dir twice should fail")
	}
	if _, err := reg.Add(filepath.Join(dir, "does-not-exist")); err == nil {
		t.Fatal("adding nonexistent dir should fail")
	}
}
