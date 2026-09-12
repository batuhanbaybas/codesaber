package git

type ChangeStatus byte

const (
	ChangeAdded     ChangeStatus = 'A'
	ChangeModified  ChangeStatus = 'M'
	ChangeDeleted   ChangeStatus = 'D'
	ChangeUntracked ChangeStatus = 'U'
)

type Change struct {
	Path   string       `json:"path"`
	Status ChangeStatus `json:"status"`
}

type Status struct {
	Branch    string   `json:"branch"`
	Staged    []Change `json:"staged"`
	Unstaged  []Change `json:"unstaged"`
	Untracked []Change `json:"untracked"`
}
