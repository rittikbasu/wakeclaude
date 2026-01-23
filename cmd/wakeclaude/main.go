package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"wakeclaude/internal/app"
	"wakeclaude/internal/tui"
)

func main() {
	fs := flag.NewFlagSet("wakeclaude", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var projectsRoot string
	var showHelp bool
	fs.StringVar(&projectsRoot, "projects-root", "", "Root directory for Claude projects (default: ~/.claude/projects)")
	fs.BoolVar(&showHelp, "help", false, "Show help")
	fs.BoolVar(&showHelp, "h", false, "Show help")

	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	if showHelp {
		printUsage()
		return
	}
	if fs.NArg() > 0 {
		fmt.Fprintln(os.Stderr, "wakeclaude does not accept positional arguments.")
		printUsage()
		os.Exit(2)
	}

	projects, err := app.DiscoverProjects(projectsRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(projects) == 0 {
		rootHint := projectsRoot
		if rootHint == "" {
			rootHint = "~/.claude/projects"
		}
		fmt.Fprintf(os.Stderr, "No Claude projects found under %s. Run Claude once or pass --projects-root.\n", rootHint)
		os.Exit(1)
	}

	models := []app.ModelOption{
		{Label: "Default (auto)", Value: "auto"},
		{Label: "Sonnet", Value: "sonnet"},
		{Label: "Opus", Value: "opus"},
		{Label: "Haiku", Value: "haiku"},
	}
	result, err := tui.Run(projects, models)
	if err != nil {
		if errors.Is(err, tui.ErrUserQuit) {
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if result.NewSession {
		fmt.Fprintf(os.Stdout, "new\t%s\t%s\n", result.ProjectPath, result.Model)
		return
	}

	fmt.Fprintf(os.Stdout, "%s\t%s\t%s\n", result.SessionID, result.SessionPath, result.Model)
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "wakeclaude - pick a local Claude session interactively")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  wakeclaude [--projects-root <path>]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --projects-root   Root directory for Claude projects (default: ~/.claude/projects)")
	fmt.Fprintln(os.Stderr, "  --help, -h        Show help")
}
