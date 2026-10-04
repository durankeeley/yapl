package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"yapl/internal/command"
	"yapl/internal/config"
	"yapl/internal/detect"
	"yapl/internal/fs"
)

// portableGamesDir is where `install --from` copies a game, inside the prefix.
const portableGamesDir = "drive_c/Games"

// maxListedCandidates keeps the detection report readable.
const maxListedCandidates = 10

// InstallOptions selects how a game gets into the prefix. Exactly one of
// Installer or From is required; Executable skips detection.
type InstallOptions struct {
	Installer  string
	From       string
	Executable string
}

// Install puts a game into the prefix, by running an installer or copying a
// portable folder, then records its executable in the config.
func (a *App) Install(opts InstallOptions, w io.Writer) error {
	if (opts.Installer == "") == (opts.From == "") {
		return errors.New("install needs exactly one of an installer or a folder to copy")
	}
	absPrefix, err := fs.GetAbsolutePath(a.PrefixPath)
	if err != nil {
		return fmt.Errorf("could not resolve prefix path: %w", err)
	}
	if !command.PrefixIsInitialized(absPrefix) {
		if err := a.Setup(); err != nil {
			return err
		}
	}

	before, err := detect.Snapshot(absPrefix)
	if err != nil {
		return fmt.Errorf("scan prefix: %w", err)
	}
	if opts.From != "" {
		err = a.copyPortable(opts.From, absPrefix, w)
	} else {
		err = a.runInstaller(opts.Installer, w)
	}
	if err != nil {
		return err
	}
	after, err := detect.Snapshot(absPrefix)
	if err != nil {
		return fmt.Errorf("scan prefix: %w", err)
	}
	return a.chooseExecutable(opts.Executable, absPrefix, detect.Candidates(before, after, a.Name), w)
}

func (a *App) copyPortable(src, absPrefix string, w io.Writer) error {
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory; use --installer for setup programs", src)
	}
	dest := filepath.Join(absPrefix, filepath.FromSlash(portableGamesDir), a.Name)
	fmt.Fprintf(w, "-> Copying %s to %s...\n", src, dest)
	if err := fs.CopyDir(src, dest); err != nil {
		return fmt.Errorf("copy game folder: %w", err)
	}
	return nil
}

func (a *App) runInstaller(installer string, w io.Writer) error {
	abs, err := filepath.Abs(installer)
	if err != nil {
		return err
	}
	if _, err := os.Stat(abs); err != nil {
		return fmt.Errorf("installer %s: %w", installer, err)
	}
	fmt.Fprintf(w, "-> Running installer %s...\n", abs)
	cfg := a.AppConfig
	cfg.Executable = abs
	cfg.LaunchArgs = nil
	cfg.BackgroundServices = nil
	if err := a.launch(cfg); err != nil {
		return fmt.Errorf("installer failed: %w", err)
	}
	if launchMethod(cfg) == "direct" {
		return command.WaitForWineserver(a.PrefixPath, cfg, a.GlobalConfig)
	}
	return nil
}

func (a *App) chooseExecutable(explicit, absPrefix string, candidates []detect.Executable, w io.Writer) error {
	chosen := ""
	switch {
	case explicit != "":
		if _, err := os.Stat(filepath.Join(absPrefix, filepath.FromSlash(explicit))); err != nil {
			return fmt.Errorf("executable %s not found in the prefix", explicit)
		}
		chosen = explicit
	case a.AppConfig.Executable == "" || a.AppConfig.Executable == config.PlaceholderExecutable:
		if len(candidates) > 0 {
			chosen = candidates[0].Path
		}
	}

	printCandidates(w, candidates, chosen)
	if chosen == "" {
		if len(candidates) == 0 && explicit == "" {
			fmt.Fprintf(w, "-> No new executable found. Set \"executable\" in %s yourself.\n", a.ConfigFile)
		} else {
			fmt.Fprintf(w, "-> Kept executable %s from %s.\n", a.AppConfig.Executable, a.ConfigFile)
		}
		return nil
	}
	a.AppConfig.Executable = chosen
	if err := config.SaveApp(a.ConfigFile, a.AppConfig); err != nil {
		return fmt.Errorf("save %s: %w", a.ConfigFile, err)
	}
	fmt.Fprintf(w, "✅ Set executable to %s in %s\n", chosen, a.ConfigFile)
	return nil
}

func printCandidates(w io.Writer, candidates []detect.Executable, chosen string) {
	if len(candidates) == 0 {
		return
	}
	fmt.Fprintln(w, "Detected executables (best match first):")
	for i, c := range candidates {
		if i == maxListedCandidates {
			fmt.Fprintf(w, "  … and %d more\n", len(candidates)-maxListedCandidates)
			break
		}
		mark := " "
		if c.Path == chosen {
			mark = "*"
		}
		fmt.Fprintf(w, "  %s %s\n", mark, c.Path)
	}
}
