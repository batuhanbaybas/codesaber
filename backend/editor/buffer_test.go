package editor

import "testing"

func TestBuffersKeepUnsavedContentPerProject(t *testing.T) {
	svc := New()
	first := svc.OpenBuffer("one", "/shared/file.go", "saved")
	svc.OpenBuffer("two", "/shared/file.go", "saved")
	first.Content = "unsaved"
	first.Version++
	if err := svc.UpdateBuffer("one", "/shared/file.go", first); err != nil {
		t.Fatal(err)
	}
	if got := svc.OpenBuffer("one", "/shared/file.go", "disk changed"); got != first {
		t.Fatalf("repeated open replaced unsaved buffer: %+v", got)
	}
	other, _ := svc.Buffer("two", "/shared/file.go")
	if other.Content != "saved" || other.ID == first.ID {
		t.Fatalf("projects share buffer state: %+v", other)
	}
	svc.RemoveProject("one")
	if _, ok := svc.Buffer("one", "/shared/file.go"); ok {
		t.Fatal("removed project retained its buffer")
	}
	if _, ok := svc.Buffer("two", "/shared/file.go"); !ok {
		t.Fatal("removing one project affected another")
	}
}

func TestBufferIgnoresSupersededUpdates(t *testing.T) {
	svc := New()
	b := svc.OpenBuffer("project", "/file.go", "saved")
	latest := b
	latest.Content, latest.Version = "latest", 2
	if err := svc.UpdateBuffer("project", "/file.go", latest); err != nil {
		t.Fatal(err)
	}
	b.Content, b.Version = "old", 1
	if err := svc.UpdateBuffer("project", "/file.go", b); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Buffer("project", "/file.go")
	if got != latest {
		t.Fatalf("late update replaced current text: %+v", got)
	}
}

func TestClosedBufferCannotBeResurrected(t *testing.T) {
	svc := New()
	b := svc.OpenBuffer("project", "/file.go", "saved")
	svc.CloseBuffer("project", "/file.go", b.ID)
	b.Content, b.Version = "late edit", 1
	if err := svc.UpdateBuffer("project", "/file.go", b); err == nil {
		t.Fatal("update recreated a closed buffer")
	}
	next := svc.OpenBuffer("project", "/file.go", "disk")
	svc.CloseBuffer("project", "/file.go", b.ID)
	if err := svc.UpdateBuffer("project", "/file.go", b); err == nil {
		t.Fatal("old tab updated a reopened buffer")
	}
	if got, ok := svc.Buffer("project", "/file.go"); !ok || got != next {
		t.Fatalf("late close removed reopened buffer: %+v, %v", got, ok)
	}
}
