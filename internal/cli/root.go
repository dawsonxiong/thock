// Package cli wires the command tree. The interactive test is the root
// command, so the common case is simply running thock with no arguments.
package cli

import (
	"context"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/fang"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/dawsonxiong/thock/internal/config"
	"github.com/dawsonxiong/thock/internal/content"
	"github.com/dawsonxiong/thock/internal/theme"
	"github.com/dawsonxiong/thock/internal/ui"
)

// version is overridable at build time with -ldflags.
var version = "0.1.0"

type flags struct {
	mode     string
	duration int
	words    int
	list     string
	length   string
	themeNam string
	noColour bool
}

// Execute runs the command tree.
func Execute(ctx context.Context) error {
	var f flags

	root := &cobra.Command{
		Use:   "thock",
		Short: "A typing test for the terminal",
		Long: "thock is a typing test for the terminal.\n\n" +
			"Run it with no arguments for a 30 second test on the thousand most\n" +
			"common English words. Speed, accuracy and consistency use the same\n" +
			"formulas as monkeytype.com, so the numbers are directly comparable.",
		Version:       version,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
		RunE:          func(cmd *cobra.Command, _ []string) error { return run(cmd, &f) },
		SilenceErrors: true,
	}

	pf := root.Flags()
	pf.StringVarP(&f.mode, "mode", "m", "", "test type: time, words or quotes")
	pf.IntVarP(&f.duration, "time", "t", 0, "seconds to type for: 15, 30, 60 or 120")
	pf.IntVarP(&f.words, "words", "w", 0, "words to type: 10, 25, 50 or 100")
	pf.StringVarP(&f.list, "list", "l", "", "word list: 1k or 5k")
	pf.StringVar(&f.length, "length", "", "quote length: any, short, medium or long")
	pf.StringVar(&f.themeNam, "theme", "", "colour theme, or 'list' to show them all")
	pf.BoolVar(&f.noColour, "no-color", false, "disable colour output")

	root.AddCommand(statsCmd(), themesCmd(), raceCmd())
	return fang.Execute(ctx, root, fang.WithVersion(version))
}

// resolve folds flags over the saved config, so an unflagged run picks up
// where the last one left off.
func resolve(f *flags) (ui.Options, config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
	}

	o := ui.Options{
		Mode:     cfg.Test.Mode,
		Duration: cfg.Test.Duration,
		Words:    cfg.Test.Words,
		List:     content.List(cfg.Test.List),
		Length:   content.Length(cfg.Test.Length),
		Theme:    cfg.Theme,
	}
	if f.mode != "" {
		o.Mode = f.mode
	}
	if f.duration != 0 {
		o.Duration = f.duration
	}
	if f.words != 0 {
		o.Words = f.words
	}
	// Naming a limit picks the mode that uses it, so "thock --words 50" needs
	// no second flag. An explicit --mode always wins.
	if f.mode == "" {
		switch {
		case f.words != 0 && f.duration == 0:
			o.Mode = ui.ModeWords
		case f.duration != 0:
			o.Mode = ui.ModeTime
		}
	}
	if f.list != "" {
		o.List = content.List(f.list)
	}
	if f.length != "" {
		o.Length = content.Length(f.length)
	}
	if f.themeNam != "" {
		o.Theme = f.themeNam
	}
	o.Colour = colourEnabled(f.noColour)
	o.Name = racerName("", cfg)

	switch o.Mode {
	case ui.ModeTime, ui.ModeWords, ui.ModeQuotes:
	default:
		return o, cfg, fmt.Errorf("unknown mode %q: use time, words or quotes", o.Mode)
	}
	if o.List != content.List1k && o.List != content.List5k {
		return o, cfg, fmt.Errorf("unknown word list %q: use 1k or 5k", o.List)
	}
	if _, ok := theme.Get(o.Theme); !ok {
		return o, cfg, fmt.Errorf("unknown theme %q: run 'thock themes' to see them", o.Theme)
	}
	switch o.Duration {
	case 15, 30, 60, 120:
	default:
		return o, cfg, fmt.Errorf("unsupported time %d: use 15, 30, 60 or 120", o.Duration)
	}
	switch o.Words {
	case 10, 25, 50, 100:
	default:
		return o, cfg, fmt.Errorf("unsupported word count %d: use 10, 25, 50 or 100", o.Words)
	}
	return o, cfg, nil
}

// colourEnabled honours both the flag and NO_COLOR, and drops colour when
// output is not a terminal so piped output stays clean.
func colourEnabled(noColour bool) bool {
	if noColour {
		return false
	}
	if _, set := os.LookupEnv("NO_COLOR"); set {
		return false
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}

func run(cmd *cobra.Command, f *flags) error {
	if f.themeNam == "list" {
		return listThemes(cmd)
	}
	opts, cfg, err := resolve(f)
	if err != nil {
		return err
	}
	m := ui.New(opts, cfg)
	p := tea.NewProgram(m, tea.WithContext(cmd.Context()), tea.WithFPS(120))
	_, err = p.Run()
	return err
}
