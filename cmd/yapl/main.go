package main

import (
	"fmt"
	"os"

	"yapl/internal/kernel"
	"yapl/internal/modules/lifecycle"
	"yapl/internal/modules/registry"
	"yapl/internal/modules/store"
)

func modules() []kernel.Module {
	return []kernel.Module{
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
func configureGlobals(k *kernel.Kernel) {}
