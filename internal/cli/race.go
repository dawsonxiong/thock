package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"os/user"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/dawsonxiong/thock/internal/config"
	"github.com/dawsonxiong/thock/internal/lan"
	"github.com/dawsonxiong/thock/internal/race"
	"github.com/dawsonxiong/thock/internal/ui"
)

type raceFlags struct {
	name     string
	themeNam string
	noColour bool
	words    int
	quote    string
	list     string
	port     int
	headless bool
}

func raceCmd() *cobra.Command {
	var f raceFlags
	cmd := &cobra.Command{
		Use:   "race",
		Short: "Race other people on your network",
		Long: "Race other people on the same network, each in their own terminal.\n\n" +
			"With no subcommand, thock lists the rooms it can hear on the network.\n" +
			"Pick one to join, or host your own. Rooms are also joined by their\n" +
			"two-word code, like velvet-orbit, or by address.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runRace(cmd, &f, func(m *ui.Model) error { m.Browse(); return nil })
		},
	}
	pf := cmd.PersistentFlags()
	pf.StringVar(&f.name, "name", "", "the name other racers see (remembered)")
	pf.StringVar(&f.themeNam, "theme", "", "colour theme")
	pf.BoolVar(&f.noColour, "no-color", false, "disable colour output")

	host := &cobra.Command{
		Use:   "host",
		Short: "Open a room for others to join",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return hostRace(cmd, &f) },
	}
	hf := host.Flags()
	hf.IntVarP(&f.words, "words", "w", 0, "race 10, 25, 50 or 100 words")
	hf.StringVar(&f.quote, "quote", "", "race a quote: any, short, medium or long")
	hf.StringVarP(&f.list, "list", "l", "", "word list: 1k or 5k")
	hf.IntVar(&f.port, "port", 0, fmt.Sprintf("port to listen on (default: first free from %d)", lan.BasePort))
	hf.BoolVar(&f.headless, "headless", false, "run the room without racing in it")

	join := &cobra.Command{
		Use:     "join <code|address>",
		Short:   "Join a room by its code or address",
		Example: "  thock race join velvet-orbit\n  thock race join 192.168.1.20:47300",
		Args:    cobra.ExactArgs(1),
		RunE:    func(cmd *cobra.Command, args []string) error { return joinRace(cmd, &f, args[0]) },
	}

	cmd.AddCommand(host, join)
	return cmd
}

// raceOptions resolves the shared flags the same way a solo test does, then
// settles who this racer is.
func raceOptions(f *raceFlags) (ui.Options, config.Config, error) {
	sf := &flags{themeNam: f.themeNam, noColour: f.noColour, list: f.list}
	switch {
	case f.quote != "":
		sf.mode, sf.length = ui.ModeQuotes, f.quote
	case f.words != 0:
		sf.mode, sf.words = ui.ModeWords, f.words
	}
	opts, cfg, err := resolve(sf)
	if err != nil {
		return opts, cfg, err
	}

	opts.Name = racerName(f.name, cfg)
	if f.name != "" && opts.Name != cfg.Race.Name {
		cfg.Race.Name = opts.Name
		if err := config.Save(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "warning: %v\n", err)
		}
	}
	return opts, cfg, nil
}

// racerName is who this racer is: the flag, then the saved name, then the
// login name.
func racerName(flag string, cfg config.Config) string {
	name := flag
	if name == "" {
		name = cfg.Race.Name
	}
	if name == "" {
		if u, err := user.Current(); err == nil {
			name = u.Username
		}
	}
	return race.CleanName(name)
}

func runRace(cmd *cobra.Command, f *raceFlags, seat func(*ui.Model) error) error {
	opts, cfg, err := raceOptions(f)
	if err != nil {
		return err
	}
	m := ui.New(opts, cfg)
	if err := seat(m); err != nil {
		return err
	}
	defer m.Close()
	p := tea.NewProgram(m, tea.WithContext(cmd.Context()), tea.WithFPS(120))
	_, err = p.Run()
	return err
}

func hostRace(cmd *cobra.Command, f *raceFlags) error {
	opts, cfg, err := raceOptions(f)
	if err != nil {
		return err
	}
	m := ui.New(opts, cfg)
	setup := m.RaceSetup()
	if f.quote != "" && setup.Length != f.quote {
		return fmt.Errorf("unknown quote length %q: use any, short, medium or long", f.quote)
	}
	if f.words != 0 && setup.Words != f.words {
		return fmt.Errorf("unsupported race of %d words: use 10, 25, 50 or 100", f.words)
	}

	if f.headless {
		return headless(cmd, opts.Name, setup, f.port)
	}
	srv, err := lan.Host(lan.HostConfig{Name: opts.Name, Setup: setup, Text: lan.Text, Port: f.port, Beacon: true})
	if err != nil {
		return err
	}
	cl, err := lan.Dial(cmd.Context(), srv.LocalAddr(), opts.Name, srv.Token())
	if err != nil {
		srv.Close()
		return err
	}
	return runRace(cmd, f, func(m *ui.Model) error { m.Join(cl, srv); return nil })
}

// headless runs a room with no screen, for a spare machine to hold it open.
// Whoever joins first cannot start rounds without the host's screen, so a
// headless room hands the start to its first guest.
func headless(cmd *cobra.Command, name string, setup race.Setup, port int) error {
	srv, err := lan.Host(lan.HostConfig{Name: name, Setup: setup, Text: lan.Text, Port: port, Beacon: true, Headless: true})
	if err != nil {
		return err
	}
	defer srv.Close()
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "room open on port %d\n", srv.Port())
	if srv.Code() != "" {
		fmt.Fprintf(out, "join with: thock race join %s\n", srv.Code())
	}
	fmt.Fprintln(out, "ctrl+c closes it")
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer stop()
	<-ctx.Done()
	return nil
}

func joinRace(cmd *cobra.Command, f *raceFlags, target string) error {
	opts, _, err := raceOptions(f)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
	defer cancel()
	addr, err := lan.Resolve(ctx, target, nil, 1500*time.Millisecond)
	if err != nil {
		return err
	}
	cl, err := lan.Dial(ctx, addr, opts.Name, "")
	if err != nil {
		return err
	}
	return runRace(cmd, f, func(m *ui.Model) error { m.Join(cl, nil); return nil })
}
