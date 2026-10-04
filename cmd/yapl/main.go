package main

import (
	"flag"
	"fmt"
	"os"

	"yapl/internal/catalog"
	"yapl/internal/kernel"
	"yapl/internal/modules/console"
	"yapl/internal/modules/lifecycle"
	"yapl/internal/modules/registry"
	"yapl/internal/modules/store"
	"yapl/internal/paths"
)

func modules() []kernel.Module {
	return []kernel.Module{
		catalog.Module{},
		console.Module{},
		lifecycle.Module{},
		registry.Module{},
		store.Module{},
	}
}

func main() {
	k := kernel.New("yapl", os.Stdout, os.Stderr)
	for _, m := range modules() {
		if err := k.Register(m); err != nil {
			fmt.Fprintf(os.Stderr, "internal error: %v\n", err)
			os.Exit(kernel.ExitError)
		}
	}
	configureGlobals(k)
	os.Exit(k.Execute(os.Args[1:]))
}

// configureGlobals registers flags accepted before the command.
func configureGlobals(k *kernel.Kernel) {
	k.AddGlobalFlags(
		func(fs *flag.FlagSet) {
			fs.String(workdirFlag, "", "Library `path` to use instead of the current directory (default $"+paths.HomeEnv+").")
		},
		func(fs *flag.FlagSet) error {
			return enterLibrary(fs.Lookup(workdirFlag).Value.String())
		},
	)
}

const workdirFlag = "workdir"

// enterLibrary makes the library root the working directory, so every
// relative path in paths resolves inside it.
func enterLibrary(flagValue string) error {
	root := flagValue
	if root == "" {
		root = os.Getenv(paths.HomeEnv)
	}
	if root == "" {
		return nil
	}
	if err := os.Chdir(root); err != nil {
		return fmt.Errorf("cannot use workdir %q: %w", root, err)
	}
	return nil
}
