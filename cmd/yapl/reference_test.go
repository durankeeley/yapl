package main

import (
	"bytes"
	"flag"
	"io"
	"os"
	"testing"

	"yapl/internal/kernel"
)

var updateReference = flag.Bool("update", false, "rewrite docs/cli-reference.md from the command registrations")

const referencePath = "../../docs/cli-reference.md"

func TestCLIReference(t *testing.T) {
	// Given every module main registers
	k := kernel.New("yapl", io.Discard, io.Discard)
	for _, m := range modules() {
		if err := k.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	configureGlobals(k)

	// When the reference is generated
	var buf bytes.Buffer
	k.WriteReference(&buf)

	if *updateReference {
		if err := os.WriteFile(referencePath, buf.Bytes(), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Then the committed docs match the binary's help exactly
	committed, err := os.ReadFile(referencePath)
	if err != nil {
		t.Fatalf("read %s: %v (run: go test ./cmd/yapl -run TestCLIReference -update)", referencePath, err)
	}
	if !bytes.Equal(committed, buf.Bytes()) {
		t.Fatal("docs/cli-reference.md is out of date; run: go test ./cmd/yapl -run TestCLIReference -update")
	}
}
