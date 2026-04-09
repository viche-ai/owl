package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migration utilities for owl configuration",
}

var migrateTemplatesCmd = &cobra.Command{
	Use:   "templates",
	Short: "Convert legacy template JSON files to agent definition format",
	Run: func(cmd *cobra.Command, args []string) {
		home, _ := os.UserHomeDir()
		cwd, _ := os.Getwd()

		type source struct {
			scope   string
			dir     string
			outBase string
		}

		sources := []source{
			{"global", filepath.Join(home, ".owl", "templates"), filepath.Join(home, ".owl", "agents")},
			{"project", filepath.Join(cwd, ".owl", "templates"), filepath.Join(cwd, ".owl", "agents")},
		}

		converted := 0
		skipped := 0

		for _, src := range sources {
			entries, err := os.ReadDir(src.dir)
			if err != nil {
				continue
			}

			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
					continue
				}

				tmplPath := filepath.Join(src.dir, e.Name())
				b, err := os.ReadFile(tmplPath)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  skip %s: read error: %v\n", tmplPath, err)
					skipped++
					continue
				}

				var tmpl Template
				if err := json.Unmarshal(b, &tmpl); err != nil {
					fmt.Fprintf(os.Stderr, "  skip %s: parse error: %v\n", tmplPath, err)
					skipped++
					continue
				}

				name := strings.TrimSuffix(e.Name(), ".json")
				if tmpl.Name != "" {
					name = tmpl.Name
				}

				agentDir := filepath.Join(src.outBase, name)
				if err := os.MkdirAll(agentDir, 0755); err != nil {
					fmt.Fprintf(os.Stderr, "  skip %s: mkdir error: %v\n", agentDir, err)
					skipped++
					continue
				}

				// Build AGENTS.md content from system_prompt + description
				var agentsMD strings.Builder
				agentsMD.WriteString("# " + name + "\n\n")
				if tmpl.Description != "" {
					agentsMD.WriteString(tmpl.Description + "\n\n")
				}
				if tmpl.SystemPrompt != "" {
					agentsMD.WriteString("## System Prompt\n\n")
					agentsMD.WriteString(tmpl.SystemPrompt + "\n")
				}

				agentsMDPath := filepath.Join(agentDir, "AGENTS.md")
				if err := os.WriteFile(agentsMDPath, []byte(agentsMD.String()), 0644); err != nil {
					fmt.Fprintf(os.Stderr, "  skip %s: write error: %v\n", agentsMDPath, err)
					skipped++
					continue
				}

				// Build agent.yaml sidecar
				var yamlLines []string
				yamlLines = append(yamlLines, "name: "+name)
				if tmpl.Model != "" {
					yamlLines = append(yamlLines, "default_model: "+tmpl.Model)
				}
				if tmpl.Effort != "" {
					yamlLines = append(yamlLines, "effort: "+tmpl.Effort)
				}
				if tmpl.Thinking {
					yamlLines = append(yamlLines, "thinking: true")
				}
				if len(tmpl.Capabilities) > 0 {
					yamlLines = append(yamlLines, "capabilities:")
					for _, cap := range tmpl.Capabilities {
						yamlLines = append(yamlLines, "  - "+cap)
					}
				}
				agentYAMLPath := filepath.Join(agentDir, "agent.yaml")
				if err := os.WriteFile(agentYAMLPath, []byte(strings.Join(yamlLines, "\n")+"\n"), 0644); err != nil {
					fmt.Fprintf(os.Stderr, "  warn: could not write agent.yaml: %v\n", err)
				}

				fmt.Printf("  [%s] %s -> %s\n", src.scope, tmplPath, agentDir)
				converted++
			}
		}

		fmt.Printf("\nConversion complete: %d converted, %d skipped\n", converted, skipped)
		if converted > 0 {
			fmt.Println("Use 'owl hatch --agent <name>' instead of 'owl hatch --template <name>'")
		}
	},
}

var migrateCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Check for unconverted templates and missing agent definition fields",
	Run: func(cmd *cobra.Command, args []string) {
		home, _ := os.UserHomeDir()
		cwd, _ := os.Getwd()

		type source struct {
			scope string
			dir   string
		}

		sources := []source{
			{"global", filepath.Join(home, ".owl", "templates")},
			{"project", filepath.Join(cwd, ".owl", "templates")},
		}

		found := 0
		for _, src := range sources {
			entries, err := os.ReadDir(src.dir)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
					continue
				}
				fmt.Printf("  [%s] unconverted: %s\n", src.scope, filepath.Join(src.dir, e.Name()))
				found++
			}
		}

		if found == 0 {
			fmt.Println("No unconverted templates found.")
		} else {
			fmt.Printf("\n%d unconverted template(s) found. Run 'owl migrate templates' to convert.\n", found)
		}

		// Check agent definitions for required fields
		agentDirs := []source{
			{"global", filepath.Join(home, ".owl", "agents")},
			{"project", filepath.Join(cwd, ".owl", "agents")},
		}

		issues := 0
		for _, src := range agentDirs {
			entries, err := os.ReadDir(src.dir)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				agentsMD := filepath.Join(src.dir, e.Name(), "AGENTS.md")
				if _, err := os.Stat(agentsMD); os.IsNotExist(err) {
					fmt.Printf("  [%s] %s: missing AGENTS.md\n", src.scope, e.Name())
					issues++
				}
			}
		}

		if issues == 0 && found == 0 {
			fmt.Println("All agent definitions look good.")
		}
	},
}

func init() {
	migrateCmd.AddCommand(migrateTemplatesCmd)
	migrateCmd.AddCommand(migrateCheckCmd)
}
