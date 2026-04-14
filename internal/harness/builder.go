package harness

import (
	"fmt"
	"os/exec"
	"strings"
)

// CheckBinary verifies the harness binary exists on PATH.
func (d *HarnessDefinition) CheckBinary() error {
	_, err := exec.LookPath(d.Binary)
	if err != nil {
		return fmt.Errorf("harness %q: binary %q not found on PATH", d.Name, d.Binary)
	}
	return nil
}

// BuildCommand expands template placeholders in the definition's Args and
// returns the binary name and final argument list ready for exec.Command.
//
// Supported placeholders:
//   - {{description}} — the task description
//   - {{workdir}}     — the working directory
//
// If WorkDirFlag is set (e.g. "--dir"), it is injected as a pair
// [flag, workDir] before the first arg containing {{description}}.
//
// extraArgs are appended after the template-expanded args.
func (d *HarnessDefinition) BuildCommand(description, workDir string, extraArgs []string) (string, []string, error) {
	var args []string
	dirInjected := false

	for _, tmpl := range d.Args {
		expanded := strings.ReplaceAll(tmpl, "{{description}}", description)
		expanded = strings.ReplaceAll(expanded, "{{workdir}}", workDir)

		// Inject workdir flag before the first arg that contained {{description}}
		if d.WorkDirFlag != "" && !dirInjected && strings.Contains(tmpl, "{{description}}") {
			args = append(args, d.WorkDirFlag, workDir)
			dirInjected = true
		}

		args = append(args, expanded)
	}

	args = append(args, extraArgs...)

	return d.Binary, args, nil
}
