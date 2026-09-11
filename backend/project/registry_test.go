package project

import (
	"path/filepath"
	"sync"
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

func TestRegistryNotifyFiresOnAddAndRemove(t *testing.T) {
	dir := t.TempDir()
	var mu sync.Mutex
	count := 0
	reg := NewRegistry(func() {
		mu.Lock()
		count++
		mu.Unlock()
	})
	p, err := reg.Add(dir)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	mu.Lock()
	if count != 1 {
		t.Fatalf("notify after Add = %d, want 1", count)
	}
	mu.Unlock()
	if err := reg.Remove(p.ID); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	mu.Lock()
	if count != 2 {
		t.Fatalf("notify after Remove = %d, want 2", count)
	}
	mu.Unlock()
}

func TestRegistryGetReturnsCopy(t *testing.T) {
	dir := t.TempDir()
	reg := NewRegistry(func() {})
	p, err := reg.Add(dir)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	got, err := reg.Get(p.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got.Name = "mutated"
	fresh, _ := reg.Get(p.ID)
	if fresh.Name == "mutated" {
		t.Fatal("Get must return a copy; mutation leaked into registry state")
	}
	if fresh.Name != filepath.Base(dir) {
		t.Fatalf("unexpected stored state: %+v", fresh)
	}
}
