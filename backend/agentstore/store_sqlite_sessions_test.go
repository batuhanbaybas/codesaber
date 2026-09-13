package agentstore

import (
	"testing"
	"time"
)

func TestSQLiteToolUpsertByToolID(t *testing.T) {
	s := newTestStore(t)
	_ = s.Append("s1", "p1", Entry{Role: "agent", Kind: KindTool, ToolID: "t-1", Text: "Reading file", Status: "in_progress"})
	_ = s.Append("s1", "p1", Entry{Role: "agent", Kind: KindTool, ToolID: "t-1", Text: "Reading file", Status: "completed"})
	entries, _ := s.Read("s1")
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1 (upsert)", len(entries))
	}
	if entries[0].Status != "completed" {
		t.Fatalf("status = %q, want completed", entries[0].Status)
	}
}

func TestSQLiteListSessionsOrderAndCount(t *testing.T) {
	s := newTestStore(t)
	_ = s.Append("s1", "p1", Entry{Role: "user", Kind: KindText, Text: "first prompt"})
	_ = s.Append("s2", "p1", Entry{Role: "user", Kind: KindText, Text: "second prompt"})
	sessions, err := s.ListSessions("p1")
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("sessions = %d, want 2", len(sessions))
	}
	// most recently updated first
	if sessions[0].ID != "s2" || sessions[1].ID != "s1" {
		t.Fatalf("order = %q,%q want s2,s1", sessions[0].ID, sessions[1].ID)
	}
	if sessions[1].Title != "first prompt" {
		t.Fatalf("title = %q, want auto-titled from first user text", sessions[1].Title)
	}
	if sessions[1].MessageCount != 1 {
		t.Fatalf("count = %d, want 1", sessions[1].MessageCount)
	}
	if sessions[1].ProjectID != "p1" {
		t.Fatalf("projectId = %q", sessions[1].ProjectID)
	}
}

func TestSQLiteListSessionsProjectIsolation(t *testing.T) {
	s := newTestStore(t)
	_ = s.Append("s1", "p1", Entry{Role: "user", Kind: KindText, Text: "a"})
	_ = s.Append("s2", "p2", Entry{Role: "user", Kind: KindText, Text: "b"})
	p1, _ := s.ListSessions("p1")
	if len(p1) != 1 || p1[0].ID != "s1" {
		t.Fatalf("p1 sessions = %#v", p1)
	}
}

func TestSQLiteRenameSession(t *testing.T) {
	s := newTestStore(t)
	_ = s.Append("s1", "p1", Entry{Role: "user", Kind: KindText, Text: "x"})
	if err := s.RenameSession("s1", "My fix session"); err != nil {
		t.Fatalf("RenameSession: %v", err)
	}
	sessions, _ := s.ListSessions("p1")
	if sessions[0].Title != "My fix session" {
		t.Fatalf("title = %q", sessions[0].Title)
	}
}

func TestSQLiteRenameMissingSessionErrors(t *testing.T) {
	s := newTestStore(t)
	if err := s.RenameSession("ghost", "nope"); err == nil {
		t.Fatal("want error renaming missing session")
	}
}

func TestSQLiteDeleteSession(t *testing.T) {
	s := newTestStore(t)
	_ = s.Append("s1", "p1", Entry{Role: "user", Kind: KindText, Text: "x"})
	_ = s.Append("s2", "p1", Entry{Role: "user", Kind: KindText, Text: "y"})
	if err := s.DeleteSession("s1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if entries, _ := s.Read("s1"); len(entries) != 0 {
		t.Fatalf("s1 entries after delete = %d", len(entries))
	}
	sessions, _ := s.ListSessions("p1")
	if len(sessions) != 1 || sessions[0].ID != "s2" {
		t.Fatalf("sessions = %#v", sessions)
	}
}

func TestSQLiteLatestSession(t *testing.T) {
	s := newTestStore(t)
	_ = s.Append("s1", "p1", Entry{Role: "user", Kind: KindText, Text: "old"})
	_ = s.Append("s2", "p1", Entry{Role: "user", Kind: KindText, Text: "new"})
	meta, err := s.LatestSession("p1")
	if err != nil {
		t.Fatalf("LatestSession: %v", err)
	}
	if meta.ID != "s2" {
		t.Fatalf("latest = %q, want s2", meta.ID)
	}
	if _, err := s.LatestSession("p-none"); err == nil {
		t.Fatal("want error for project with no sessions")
	}
}

// A whole-second timestamp must not sort after a later fractional one even
// though lexicographically "...00Z" > "...00.5Z" under RFC3339Nano.
func TestSQLiteTimestampOrderingWholeVsFractionalSeconds(t *testing.T) {
	s := newTestStore(t)
	whole, _ := time.Parse(time.RFC3339, "2026-01-01T10:00:00Z")
	_ = s.Append("s1", "p1", Entry{Role: "user", Kind: KindText, Text: "old", When: whole})
	frac, _ := time.Parse(time.RFC3339Nano, "2026-01-01T10:00:00.5Z")
	_ = s.Append("s2", "p1", Entry{Role: "user", Kind: KindText, Text: "new", When: frac})

	sessions, err := s.ListSessions("p1")
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 2 || sessions[0].ID != "s2" || sessions[1].ID != "s1" {
		t.Fatalf("order = %v,%v want s2,s1", sessions[0].ID, sessions[1].ID)
	}
	meta, _ := s.LatestSession("p1")
	if meta.ID != "s2" {
		t.Fatalf("latest = %q, want s2", meta.ID)
	}
}

func TestSQLiteClearAllLeavesOtherProjects(t *testing.T) {
	s := newTestStore(t)
	_ = s.Append("s1", "p1", Entry{Role: "user", Kind: KindText, Text: "a"})
	_ = s.Append("s2", "p1", Entry{Role: "user", Kind: KindText, Text: "b"})
	_ = s.Append("s3", "p2", Entry{Role: "user", Kind: KindText, Text: "c"})
	if err := s.ClearAll("p1"); err != nil {
		t.Fatalf("ClearAll: %v", err)
	}
	if sessions, _ := s.ListSessions("p1"); len(sessions) != 0 {
		t.Fatalf("p1 sessions after clear = %d, want 0", len(sessions))
	}
	if entries, _ := s.Read("s1"); len(entries) != 0 {
		t.Fatalf("s1 entries after clear = %d, want 0", len(entries))
	}
	if entries, _ := s.Read("s2"); len(entries) != 0 {
		t.Fatalf("s2 entries after clear = %d, want 0", len(entries))
	}
	if sessions, _ := s.ListSessions("p2"); len(sessions) != 1 || sessions[0].ID != "s3" {
		t.Fatalf("p2 sessions = %#v, want untouched s3", sessions)
	}
}

func TestSQLiteAppendExistingSessionEmptyProjectID(t *testing.T) {
	s := newTestStore(t)
	_ = s.Append("s1", "p1", Entry{Role: "user", Kind: KindText, Text: "first"})
	if err := s.Append("s1", "", Entry{Role: "agent", Kind: KindText, Text: "second"}); err != nil {
		t.Fatalf("Append with empty projectID: %v", err)
	}
	if entries, _ := s.Read("s1"); len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
}

func TestSQLiteRenameTitleSurvivesUserAppend(t *testing.T) {
	s := newTestStore(t)
	_ = s.Append("s1", "p1", Entry{Role: "user", Kind: KindText, Text: "original"})
	if err := s.RenameSession("s1", "Renamed"); err != nil {
		t.Fatalf("RenameSession: %v", err)
	}
	_ = s.Append("s1", "p1", Entry{Role: "user", Kind: KindText, Text: "more user text"})
	sessions, _ := s.ListSessions("p1")
	if sessions[0].Title != "Renamed" {
		t.Fatalf("title = %q, want renamed title preserved", sessions[0].Title)
	}
}
