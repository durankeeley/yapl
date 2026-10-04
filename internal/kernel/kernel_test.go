package kernel

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"strings"
	"testing"
)

type fakeModule struct {
	name, version string
	commands      []Command
}

func (m fakeModule) Name() string        { return m.name }
func (m fakeModule) Version() string     { return m.version }
func (m fakeModule) Commands() []Command { return m.commands }

type harness struct {
	kernel         *Kernel
	stdout, stderr *bytes.Buffer
}

func newHarness(t *testing.T, modules ...Module) harness {
	t.Helper()
	var out, errOut bytes.Buffer
	k := New("yapl", &out, &errOut)
	for _, m := range modules {
		if err := k.Register(m); err != nil {
			t.Fatalf("register %s: %v", m.Name(), err)
		}
	}
	return harness{kernel: k, stdout: &out, stderr: &errOut}
}

func greetModule(captured *Context) fakeModule {
	return fakeModule{name: "greeter", version: "1.2.0", commands: []Command{{
		Verb:     "greet",
		Usage:    "greet <name> [flags]",
		Summary:  "Say hello",
		Examples: []string{`yapl greet "Doom" --loud`},
		Flags: func(fs *flag.FlagSet) {
			fs.Bool("loud", false, "Shout the greeting.")
			fs.String("format", "text", "Output `type`: text or json.")
			fs.Int("times", 1, "Repeat count.")
		},
		Run: func(ctx *Context) error {
			*captured = *ctx
			return nil
		},
	}}}
}

func TestExecute_DispatchesToCommand(t *testing.T) {
	t.Run("given flags after positional args when executed then both are parsed", func(t *testing.T) {
		// Given
		var ctx Context
		h := newHarness(t, greetModule(&ctx))

		// When
		code := h.kernel.Execute([]string{"greet", "Doom", "--loud", "--format", "json", "--times=3"})

		// Then
		if code != ExitOK {
			t.Fatalf("exit %d, stderr: %s", code, h.stderr)
		}
		if ctx.Arg(0) != "Doom" || !ctx.Bool("loud") || ctx.String("format") != "json" || ctx.Int("times") != 3 {
			t.Fatalf("unexpected context: args=%v loud=%v format=%q times=%d", ctx.Args, ctx.Bool("loud"), ctx.String("format"), ctx.Int("times"))
		}
	})

	t.Run("given flags before positional args when executed then both are parsed", func(t *testing.T) {
		// Given
		var ctx Context
		h := newHarness(t, greetModule(&ctx))

		// When
		code := h.kernel.Execute([]string{"greet", "--format", "json", "Doom"})

		// Then
		if code != ExitOK || ctx.Arg(0) != "Doom" || ctx.String("format") != "json" {
			t.Fatalf("exit %d args=%v format=%q", code, ctx.Args, ctx.String("format"))
		}
	})

	t.Run("given a double dash when executed then everything after it is positional", func(t *testing.T) {
		// Given
		var ctx Context
		h := newHarness(t, greetModule(&ctx))

		// When
		h.kernel.Execute([]string{"greet", "--", "--loud"})

		// Then
		if ctx.Bool("loud") || ctx.Arg(0) != "--loud" {
			t.Fatalf("args=%v loud=%v", ctx.Args, ctx.Bool("loud"))
		}
	})

	t.Run("given an out of range argument index then Arg returns empty", func(t *testing.T) {
		// Given
		var ctx Context
		h := newHarness(t, greetModule(&ctx))

		// When
		h.kernel.Execute([]string{"greet"})

		// Then
		if ctx.Arg(5) != "" {
			t.Fatal("expected empty argument")
		}
	})
}

func TestExecute_FlagsAreScopedToTheirCommand(t *testing.T) {
	// Given a second command that does not declare --loud
	var ctx Context
	other := fakeModule{name: "other", version: "1.0.0", commands: []Command{{Verb: "wave", Summary: "Wave", Run: func(*Context) error { return nil }}}}
	h := newHarness(t, greetModule(&ctx), other)

	// When
	code := h.kernel.Execute([]string{"wave", "--loud"})

	// Then
	if code != ExitUsage {
		t.Fatalf("exit %d, want usage error", code)
	}
	if !strings.Contains(h.stderr.String(), "--loud") || !strings.Contains(h.stderr.String(), "Usage: yapl wave") {
		t.Fatalf("stderr missing flag name or usage:\n%s", h.stderr)
	}
}

func TestExecute_Errors(t *testing.T) {
	t.Run("given an unknown command when executed then it is a usage error", func(t *testing.T) {
		// Given
		h := newHarness(t)

		// When
		code := h.kernel.Execute([]string{"fly"})

		// Then
		if code != ExitUsage || !strings.Contains(h.stderr.String(), `unknown command "fly"`) {
			t.Fatalf("exit %d stderr %s", code, h.stderr)
		}
	})

	t.Run("given a command that fails when executed then the error is printed with exit 1", func(t *testing.T) {
		// Given
		failing := fakeModule{name: "f", version: "1.0.0", commands: []Command{{Verb: "boom", Run: func(*Context) error { return errors.New("it broke") }}}}
		h := newHarness(t, failing)

		// When
		code := h.kernel.Execute([]string{"boom"})

		// Then
		if code != ExitError || !strings.Contains(h.stderr.String(), "error: it broke") {
			t.Fatalf("exit %d stderr %s", code, h.stderr)
		}
	})

	t.Run("given a command that returns a usage error then its usage is printed with exit 2", func(t *testing.T) {
		// Given
		bad := fakeModule{name: "f", version: "1.0.0", commands: []Command{{Verb: "need", Usage: "need <name>", Run: func(*Context) error { return UsageError("a name is required") }}}}
		h := newHarness(t, bad)

		// When
		code := h.kernel.Execute([]string{"need"})

		// Then
		if code != ExitUsage || !strings.Contains(h.stderr.String(), "a name is required") || !strings.Contains(h.stderr.String(), "Usage: yapl need <name>") {
			t.Fatalf("exit %d stderr %s", code, h.stderr)
		}
	})

	t.Run("given no arguments when executed then overall help is shown as a usage error", func(t *testing.T) {
		// Given
		var ctx Context
		h := newHarness(t, greetModule(&ctx))

		// When
		code := h.kernel.Execute(nil)

		// Then
		if code != ExitUsage || !strings.Contains(h.stderr.String(), "greet") {
			t.Fatalf("exit %d stderr %s", code, h.stderr)
		}
	})
}

func TestRegister(t *testing.T) {
	t.Run("given two modules declaring the same verb when registered then the second is rejected", func(t *testing.T) {
		// Given
		var ctx Context
		h := newHarness(t, greetModule(&ctx))

		// When
		err := h.kernel.Register(fakeModule{name: "dup", version: "1.0.0", commands: []Command{{Verb: "greet"}}})

		// Then
		if err == nil || !strings.Contains(err.Error(), "greet") || !strings.Contains(err.Error(), "greeter") {
			t.Fatalf("expected duplicate error naming verb and owner, got %v", err)
		}
	})

	t.Run("given a module reusing a built-in verb when registered then it is rejected", func(t *testing.T) {
		h := newHarness(t)
		if err := h.kernel.Register(fakeModule{name: "x", version: "1.0.0", commands: []Command{{Verb: "help"}}}); err == nil {
			t.Fatal("expected built-in verb clash to be rejected")
		}
	})

	t.Run("given a module with an invalid version when registered then it is rejected", func(t *testing.T) {
		h := newHarness(t)
		if err := h.kernel.Register(fakeModule{name: "x", version: "one"}); err == nil {
			t.Fatal("expected invalid version to be rejected")
		}
	})

	t.Run("given a module registered twice then the second registration is rejected", func(t *testing.T) {
		h := newHarness(t, fakeModule{name: "x", version: "1.0.0"})
		if err := h.kernel.Register(fakeModule{name: "x", version: "1.0.0"}); err == nil {
			t.Fatal("expected duplicate module name to be rejected")
		}
	})
}

func TestHelp(t *testing.T) {
	t.Run("given registered modules when help runs then every verb and summary is listed", func(t *testing.T) {
		// Given
		var ctx Context
		h := newHarness(t, greetModule(&ctx))

		// When
		code := h.kernel.Execute([]string{"help"})

		// Then
		out := h.stdout.String()
		if code != ExitOK || !strings.Contains(out, "greet") || !strings.Contains(out, "Say hello") || !strings.Contains(out, "version") {
			t.Fatalf("exit %d help:\n%s", code, out)
		}
	})

	t.Run("given help for one command then usage, flags with double dashes and examples are shown", func(t *testing.T) {
		// Given
		var ctx Context
		h := newHarness(t, greetModule(&ctx))

		// When
		code := h.kernel.Execute([]string{"help", "greet"})

		// Then
		out := h.stdout.String()
		for _, want := range []string{"Usage: yapl greet <name> [flags]", "--loud", "--format <type>", `default "text"`, `yapl greet "Doom" --loud`} {
			if !strings.Contains(out, want) {
				t.Errorf("command help missing %q:\n%s", want, out)
			}
		}
		if code != ExitOK {
			t.Fatalf("exit %d", code)
		}
	})

	t.Run("given --help after a verb then the command help is shown and the command does not run", func(t *testing.T) {
		// Given
		ran := false
		m := fakeModule{name: "m", version: "1.0.0", commands: []Command{{Verb: "go", Summary: "Go", Run: func(*Context) error { ran = true; return nil }}}}
		h := newHarness(t, m)

		// When
		code := h.kernel.Execute([]string{"go", "--help"})

		// Then
		if code != ExitOK || ran || !strings.Contains(h.stdout.String(), "Usage: yapl go") {
			t.Fatalf("exit %d ran=%v out=%s", code, ran, h.stdout)
		}
	})

	t.Run("given help for an unknown command then it is a usage error", func(t *testing.T) {
		h := newHarness(t)
		if code := h.kernel.Execute([]string{"help", "nope"}); code != ExitUsage {
			t.Fatalf("exit %d", code)
		}
	})
}

func TestVersionCommand(t *testing.T) {
	t.Run("given registered modules when version runs then kernel and modules are listed", func(t *testing.T) {
		// Given
		var ctx Context
		h := newHarness(t, greetModule(&ctx), fakeModule{name: "catalog", version: "2026.10.0"})

		// When
		code := h.kernel.Execute([]string{"version"})

		// Then
		out := h.stdout.String()
		for _, want := range []string{"kernel", Version, "greeter", "1.2.0", "catalog", "2026.10.0"} {
			if !strings.Contains(out, want) {
				t.Errorf("version output missing %q:\n%s", want, out)
			}
		}
		if code != ExitOK {
			t.Fatalf("exit %d", code)
		}
	})

	t.Run("given --output json when version runs then machine readable versions are printed", func(t *testing.T) {
		// Given
		var ctx Context
		h := newHarness(t, greetModule(&ctx))

		// When
		code := h.kernel.Execute([]string{"version", "--output", "json"})

		// Then
		var got Versions
		if err := json.Unmarshal(h.stdout.Bytes(), &got); err != nil {
			t.Fatalf("invalid json %q: %v", h.stdout, err)
		}
		if code != ExitOK || got.Kernel != Version || got.Modules["greeter"] != "1.2.0" {
			t.Fatalf("exit %d versions %+v", code, got)
		}
	})

	t.Run("given an unknown output format then it is a usage error", func(t *testing.T) {
		h := newHarness(t)
		if code := h.kernel.Execute([]string{"version", "--output", "xml"}); code != ExitUsage {
			t.Fatalf("exit %d", code)
		}
	})
}

func TestGlobalFlags(t *testing.T) {
	t.Run("given a registered global flag before the verb when executed then the hook sees its value", func(t *testing.T) {
		// Given
		var ctx Context
		h := newHarness(t, greetModule(&ctx))
		var seen string
		h.kernel.AddGlobalFlags(func(fs *flag.FlagSet) { fs.String("workdir", "", "Library `path`.") }, func(fs *flag.FlagSet) error {
			seen = fs.Lookup("workdir").Value.String()
			return nil
		})

		// When
		code := h.kernel.Execute([]string{"--workdir", "/srv/games", "greet", "Doom"})

		// Then
		if code != ExitOK || seen != "/srv/games" || ctx.Arg(0) != "Doom" {
			t.Fatalf("exit %d seen=%q args=%v stderr=%s", code, seen, ctx.Args, h.stderr)
		}
	})

	t.Run("given a failing global hook when executed then the command does not run", func(t *testing.T) {
		// Given
		ran := false
		m := fakeModule{name: "m", version: "1.0.0", commands: []Command{{Verb: "go", Run: func(*Context) error { ran = true; return nil }}}}
		h := newHarness(t, m)
		h.kernel.AddGlobalFlags(func(*flag.FlagSet) {}, func(*flag.FlagSet) error { return errors.New("bad workdir") })

		// When
		code := h.kernel.Execute([]string{"go"})

		// Then
		if code != ExitError || ran || !strings.Contains(h.stderr.String(), "bad workdir") {
			t.Fatalf("exit %d ran=%v stderr=%s", code, ran, h.stderr)
		}
	})

	t.Run("given global flags when help runs then they are listed", func(t *testing.T) {
		h := newHarness(t)
		h.kernel.AddGlobalFlags(func(fs *flag.FlagSet) { fs.String("workdir", "", "Library `path`.") }, nil)
		h.kernel.Execute([]string{"help"})
		if !strings.Contains(h.stdout.String(), "--workdir <path>") {
			t.Fatalf("help missing global flag:\n%s", h.stdout)
		}
	})
}

func TestWriteReference(t *testing.T) {
	t.Run("given registered commands when the reference is written then each command has usage, flags and examples in markdown", func(t *testing.T) {
		// Given
		var ctx Context
		h := newHarness(t, greetModule(&ctx))
		var buf bytes.Buffer

		// When
		h.kernel.WriteReference(&buf)

		// Then
		out := buf.String()
		for _, want := range []string{"## `yapl greet`", "yapl greet <name> [flags]", "| `--format <type>` |", "| `--loud` |", `yapl greet "Doom" --loud`, "## `yapl version`"} {
			if !strings.Contains(out, want) {
				t.Errorf("reference missing %q:\n%s", want, out)
			}
		}
	})
}
