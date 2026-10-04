// Package kernel is YAPL's microkernel. It owns argument parsing, help,
// version reporting and dispatch; every feature lives in a Module that
// declares its own name, semantic version and commands.
package kernel

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"

	"yapl/internal/version"
)

// Version is the kernel's own version. Bump it when the Module or Command
// contract changes.
const Version = "1.0.0"

const (
	ExitOK    = 0
	ExitError = 1
	ExitUsage = 2
)

// Module is an independently versioned unit of YAPL functionality. A module
// with no commands still contributes its version (for example the defaults
// catalog), which `yapl version` and the registry report.
type Module interface {
	Name() string
	Version() string
	Commands() []Command
}

// Command is one CLI verb contributed by a module.
type Command struct {
	Verb        string
	Usage       string
	Summary     string
	Description string
	Examples    []string
	Flags       func(fs *flag.FlagSet)
	Run         func(ctx *Context) error
}

// Versions is the machine-readable form of `yapl version --output json`.
type Versions struct {
	Kernel  string            `json:"kernel"`
	Modules map[string]string `json:"modules"`
}

type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg }

// UsageError marks an error as a misuse of the command, so the kernel prints
// the command's usage and exits with ExitUsage.
func UsageError(format string, args ...any) error {
	return usageError{msg: fmt.Sprintf(format, args...)}
}

type registeredCommand struct {
	Command
	module string
}

type globalFlags struct {
	define func(fs *flag.FlagSet)
	apply  func(fs *flag.FlagSet) error
}

// Kernel dispatches command lines to registered module commands.
type Kernel struct {
	program  string
	stdout   io.Writer
	stderr   io.Writer
	modules  []Module
	commands map[string]registeredCommand
	order    []string
	globals  []globalFlags
}

// New creates a kernel with the built-in help and version commands.
func New(program string, stdout, stderr io.Writer) *Kernel {
	k := &Kernel{program: program, stdout: stdout, stderr: stderr, commands: map[string]registeredCommand{}}
	for _, c := range k.builtins() {
		k.commands[c.Verb] = registeredCommand{Command: c, module: "kernel"}
		k.order = append(k.order, c.Verb)
	}
	return k
}

// Register adds a module and its commands. Verbs must be unique.
func (k *Kernel) Register(m Module) error {
	if !version.Valid(m.Version()) {
		return fmt.Errorf("module %q has invalid version %q", m.Name(), m.Version())
	}
	for _, existing := range k.modules {
		if existing.Name() == m.Name() {
			return fmt.Errorf("module %q is already registered", m.Name())
		}
	}
	for _, c := range m.Commands() {
		if owner, ok := k.commands[c.Verb]; ok {
			return fmt.Errorf("module %q cannot register %q: already provided by %q", m.Name(), c.Verb, owner.module)
		}
	}
	k.modules = append(k.modules, m)
	for _, c := range m.Commands() {
		k.commands[c.Verb] = registeredCommand{Command: c, module: m.Name()}
		k.order = append(k.order, c.Verb)
	}
	return nil
}

// AddGlobalFlags registers flags accepted before the verb, and a hook that
// runs with their parsed values before any command.
func (k *Kernel) AddGlobalFlags(define func(fs *flag.FlagSet), apply func(fs *flag.FlagSet) error) {
	k.globals = append(k.globals, globalFlags{define: define, apply: apply})
}

// Versions reports the kernel and every registered module's version.
func (k *Kernel) Versions() Versions {
	v := Versions{Kernel: Version, Modules: map[string]string{}}
	for _, m := range k.modules {
		v.Modules[m.Name()] = m.Version()
	}
	return v
}

// Execute runs a command line (without the program name) and returns the
// process exit code.
func (k *Kernel) Execute(args []string) int {
	globalSet := k.newGlobalFlagSet()
	if err := globalSet.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			k.printOverview(k.stdout)
			return ExitOK
		}
		fmt.Fprintf(k.stderr, "error: %v\n\n", doubleDash(err))
		k.printOverview(k.stderr)
		return ExitUsage
	}
	rest := globalSet.Args()
	if len(rest) == 0 {
		k.printOverview(k.stderr)
		return ExitUsage
	}

	cmd, ok := k.commands[rest[0]]
	if !ok {
		fmt.Fprintf(k.stderr, "error: unknown command %q\n\n", rest[0])
		k.printOverview(k.stderr)
		return ExitUsage
	}

	fs := newCommandFlagSet(cmd.Command)
	if err := fs.Parse(reorderArgs(fs, rest[1:])); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			k.printCommandHelp(k.stdout, cmd.Command)
			return ExitOK
		}
		fmt.Fprintf(k.stderr, "error: %v\n\n", doubleDash(err))
		k.printCommandHelp(k.stderr, cmd.Command)
		return ExitUsage
	}

	for _, g := range k.globals {
		if g.apply == nil {
			continue
		}
		if err := g.apply(globalSet); err != nil {
			fmt.Fprintf(k.stderr, "error: %v\n", err)
			return ExitError
		}
	}

	ctx := &Context{Args: fs.Args(), Flags: fs, Stdout: k.stdout, Stderr: k.stderr, Kernel: k}
	if err := cmd.Run(ctx); err != nil {
		var ue usageError
		if errors.As(err, &ue) {
			fmt.Fprintf(k.stderr, "error: %v\n\n", err)
			k.printCommandHelp(k.stderr, cmd.Command)
			return ExitUsage
		}
		fmt.Fprintf(k.stderr, "error: %v\n", err)
		return ExitError
	}
	return ExitOK
}

func (k *Kernel) newGlobalFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet(k.program, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	for _, g := range k.globals {
		g.define(fs)
	}
	return fs
}

func newCommandFlagSet(c Command) *flag.FlagSet {
	fs := flag.NewFlagSet(c.Verb, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if c.Flags != nil {
		c.Flags(fs)
	}
	return fs
}

// reorderArgs moves flags (and their values) ahead of positional arguments so
// flags may be written anywhere on the command line.
func reorderArgs(fs *flag.FlagSet, args []string) []string {
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positional = append(positional, arg)
			continue
		}
		flags = append(flags, arg)
		if strings.Contains(arg, "=") || i+1 >= len(args) {
			continue
		}
		if f := fs.Lookup(strings.TrimLeft(arg, "-")); f != nil && !isBoolFlag(f) {
			i++
			flags = append(flags, args[i])
		}
	}
	return append(append(flags, "--"), positional...)
}

// doubleDash rewrites the flag package's single-dash flag names in errors
// ("-loud") to the double-dash form YAPL documents ("--loud").
func doubleDash(err error) error {
	msg := err.Error()
	if i := strings.LastIndex(msg, ": -"); i >= 0 {
		msg = msg[:i] + ": --" + msg[i+3:]
	}
	return errors.New(msg)
}

func isBoolFlag(f *flag.Flag) bool {
	bf, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && bf.IsBoolFlag()
}

func (k *Kernel) sortedModules() []Module {
	mods := append([]Module{}, k.modules...)
	sort.Slice(mods, func(i, j int) bool { return mods[i].Name() < mods[j].Name() })
	return mods
}
