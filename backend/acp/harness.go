package acp

import (
	"errors"
	"fmt"
	"os/exec"
)

// Info describes one launchable ACP agent profile.
type Info struct {
	Name      string   `json:"name"`
	Command   []string `json:"command"`
	Available bool     `json:"available"`
}

// ACP launch commands per agent, documented on https://agentclientprotocol.com:
//   - opencode: native ACP support, `opencode acp`
//   - claude: via the claude-code-acp adapter binary
//   - gemini: `gemini --experimental-acp`
var profileCommands = []Info{
	{Name: "opencode", Command: []string{"opencode", "acp"}},
	{Name: "claude", Command: []string{"claude-code-acp"}},
	{Name: "gemini", Command: []string{"gemini", "--experimental-acp"}},
}

// DefaultProfiles returns the known agent profiles with Available set by an
// exec.LookPath check on the command's first element.
func DefaultProfiles() []Info {
	out := make([]Info, 0, len(profileCommands))
	for _, p := range profileCommands {
		p.Available = lookupAvailable(p.Command)
		out = append(out, p)
	}
	return out
}

// Resolve returns the profile for name with its availability refreshed, or an
// error if the agent binary is not on PATH.
func Resolve(name string) (*Info, error) {
	for _, p := range profileCommands {
		if p.Name != name {
			continue
		}
		if !lookupAvailable(p.Command) {
			return nil, fmt.Errorf("acp: agent %q not found on PATH", name)
		}
		profile := p
		profile.Available = true
		return &profile, nil
	}
	return nil, errors.New("acp: unknown agent profile " + name)
}

func lookupAvailable(command []string) bool {
	if len(command) == 0 {
		return false
	}
	_, err := exec.LookPath(command[0])
	return err == nil
}
