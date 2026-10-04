package kernel

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

const (
	outputText = "text"
	outputJSON = "json"
)

func (k *Kernel) builtins() []Command {
	return []Command{
		{
			Verb:     "help",
			Usage:    "help [command]",
			Summary:  "Show help for YAPL or for one command",
			Examples: []string{"yapl help", "yapl help run"},
			Run: func(ctx *Context) error {
				verb := ctx.Arg(0)
				if verb == "" {
					k.printOverview(ctx.Stdout)
					return nil
				}
				cmd, ok := k.commands[verb]
				if !ok {
					return UsageError("unknown command %q", verb)
				}
				k.printCommandHelp(ctx.Stdout, cmd.Command)
				return nil
			},
		},
		{
			Verb:        "version",
			Usage:       "version [flags]",
			Summary:     "Show the version of YAPL and each of its modules",
			Description: "Every part of YAPL is versioned independently. LAN servers and clients use these versions to decide what needs upgrading.",
			Examples:    []string{"yapl version", "yapl version --output json"},
			Flags: func(fs *flag.FlagSet) {
				fs.String("output", outputText, "Output `format`: text or json.")
			},
			Run: func(ctx *Context) error {
				return k.printVersions(ctx.Stdout, ctx.String("output"))
			},
		},
	}
}

func (k *Kernel) printVersions(w io.Writer, format string) error {
	switch format {
	case outputJSON:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(k.Versions())
	case outputText:
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "COMPONENT\tVERSION")
		fmt.Fprintf(tw, "kernel\t%s\n", Version)
		for _, m := range k.sortedModules() {
			fmt.Fprintf(tw, "%s\t%s\n", m.Name(), m.Version())
		}
		return tw.Flush()
	default:
		return UsageError("unknown output format %q: use text or json", format)
	}
}

func (k *Kernel) printOverview(w io.Writer) {
	fmt.Fprintf(w, "YAPL — Yet Another Proton Launcher\n\n")
	fmt.Fprintf(w, "Usage: %s [global flags] <command> [arguments] [flags]\n\n", k.program)
	fmt.Fprintln(w, "Commands:")
	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	for _, verb := range k.order {
		fmt.Fprintf(tw, "  %s\t%s\n", verb, k.commands[verb].Summary)
	}
	tw.Flush()
	if globals := k.newGlobalFlagSet(); hasFlags(globals) {
		fmt.Fprintln(w, "\nGlobal flags (before the command):")
		printFlags(w, globals)
	}
	fmt.Fprintf(w, "\nRun '%s help <command>' for a command's flags and examples.\n", k.program)
}

func (k *Kernel) printCommandHelp(w io.Writer, c Command) {
	usage := c.Usage
	if usage == "" {
		usage = c.Verb
	}
	fmt.Fprintf(w, "Usage: %s %s\n", k.program, usage)
	if c.Summary != "" {
		fmt.Fprintf(w, "\n%s\n", c.Summary)
	}
	if c.Description != "" {
		fmt.Fprintf(w, "\n%s\n", c.Description)
	}
	if fs := newCommandFlagSet(c); hasFlags(fs) {
		fmt.Fprintln(w, "\nFlags:")
		printFlags(w, fs)
	}
	if len(c.Examples) > 0 {
		fmt.Fprintln(w, "\nExamples:")
		for _, ex := range c.Examples {
			fmt.Fprintf(w, "  %s\n", ex)
		}
	}
}

func hasFlags(fs *flag.FlagSet) bool {
	n := 0
	fs.VisitAll(func(*flag.Flag) { n++ })
	return n > 0
}

// printFlags renders flags in YAPL's double-dash style; flag.PrintDefaults
// would print single dashes.
func printFlags(w io.Writer, fs *flag.FlagSet) {
	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	fs.VisitAll(func(f *flag.Flag) {
		name, usage := flag.UnquoteUsage(f)
		left := "--" + f.Name
		if name != "" && !isBoolFlag(f) {
			left += " <" + name + ">"
		}
		if def := f.DefValue; def != "" && def != "false" && def != "0" {
			if _, isString := f.Value.(flag.Getter).Get().(string); isString {
				def = fmt.Sprintf("%q", def)
			}
			usage += fmt.Sprintf(" (default %s)", def)
		}
		fmt.Fprintf(tw, "  %s\t%s\n", left, strings.TrimSpace(usage))
	})
	tw.Flush()
}

// WriteReference renders the full command reference as Markdown. The
// repository's docs/cli-reference.md is generated from it, so documentation
// and `yapl help` can never disagree.
func (k *Kernel) WriteReference(w io.Writer) {
	fmt.Fprintf(w, "# YAPL CLI Reference\n\n")
	fmt.Fprintf(w, "<!-- Generated from the command registrations. Regenerate with: go test ./cmd/yapl -run TestCLIReference -update -->\n\n")
	fmt.Fprintf(w, "Usage: `%s [global flags] <command> [arguments] [flags]`\n\n", k.program)
	if globals := k.newGlobalFlagSet(); hasFlags(globals) {
		fmt.Fprintf(w, "## Global flags\n\nPlace these before the command.\n\n")
		writeFlagTable(w, globals)
	}
	for _, verb := range k.order {
		c := k.commands[verb].Command
		usage := c.Usage
		if usage == "" {
			usage = c.Verb
		}
		fmt.Fprintf(w, "## `%s %s`\n\n%s\n\n```\n%s %s\n```\n\n", k.program, c.Verb, c.Summary, k.program, usage)
		if c.Description != "" {
			fmt.Fprintf(w, "%s\n\n", c.Description)
		}
		if fs := newCommandFlagSet(c); hasFlags(fs) {
			writeFlagTable(w, fs)
		}
		if len(c.Examples) > 0 {
			fmt.Fprintf(w, "```bash\n%s\n```\n\n", strings.Join(c.Examples, "\n"))
		}
	}
}

func writeFlagTable(w io.Writer, fs *flag.FlagSet) {
	fmt.Fprintf(w, "| Flag | Description | Default |\n|------|-------------|---------|\n")
	fs.VisitAll(func(f *flag.Flag) {
		name, usage := flag.UnquoteUsage(f)
		left := "--" + f.Name
		if name != "" && !isBoolFlag(f) {
			left += " <" + name + ">"
		}
		def := f.DefValue
		if def == "false" || def == "0" {
			def = ""
		}
		if def != "" {
			def = "`" + def + "`"
		}
		fmt.Fprintf(w, "| `%s` | %s | %s |\n", left, strings.TrimSpace(usage), def)
	})
	fmt.Fprintln(w)
}
