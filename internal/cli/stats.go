package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dawsonxiong/thock/internal/chart"
	"github.com/dawsonxiong/thock/internal/config"
	"github.com/dawsonxiong/thock/internal/history"
	"github.com/dawsonxiong/thock/internal/theme"
)

func statsCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Show personal bests and recent results",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return showStats(cmd, limit) },
	}
	cmd.Flags().IntVarP(&limit, "limit", "n", 10, "how many recent results to list")
	return cmd
}

func showStats(cmd *cobra.Command, limit int) error {
	records, err := history.Load()
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()

	cfg, _ := config.Load()
	t, ok := theme.Get(cfg.Theme)
	if !ok {
		t, _ = theme.Get(theme.Default)
	}
	tbl := theme.Resolve(t, colourEnabled(false))

	if len(records) == 0 {
		p, _ := history.Path()
		fmt.Fprintf(out, "\n  %s\n\n", paint(tbl.Dim, "no results yet — run thock to record one"))
		fmt.Fprintf(out, "  %s\n\n", paint(tbl.Dim, "results are stored at "+p))
		return nil
	}

	fmt.Fprintln(out)
	writeBests(out, records, tbl)
	writeTrend(out, records, tbl)
	writeRecent(out, records, tbl, limit)
	fmt.Fprintln(out)
	return nil
}

// writeBests shows the top speed for each comparable group. A 30 second run is
// only a best against other 30 second runs, so the groups are kept apart.
func writeBests(out interface{ Write([]byte) (int, error) }, rs []history.Record, tbl theme.Table) {
	best := history.Best(rs)
	keys := make([]string, 0, len(best))
	for k := range best {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return best[keys[i]].WPM > best[keys[j]].WPM })

	fmt.Fprintf(out, "  %s\n", paint(tbl.Dim, "personal bests"))
	var line strings.Builder
	for i, k := range keys {
		if i > 0 && i%4 == 0 {
			fmt.Fprintf(out, "  %s\n", line.String())
			line.Reset()
		}
		line.WriteString(fmt.Sprintf("  %-6s %s", k, paint(tbl.Accent, fmt.Sprintf("%-6.0f", best[k].WPM))))
	}
	if line.Len() > 0 {
		fmt.Fprintf(out, "  %s\n", line.String())
	}
	fmt.Fprintln(out)
}

// writeTrend draws every recorded run as a sparkline, so improvement over time
// is visible at a glance without reading any numbers.
func writeTrend(out interface{ Write([]byte) (int, error) }, rs []history.Record, tbl theme.Table) {
	wpms := make([]float64, len(rs))
	sum := 0.0
	for i, r := range rs {
		wpms[i] = r.WPM
		sum += r.WPM
	}
	width := 40
	if len(wpms) < width {
		width = len(wpms)
	}
	if width < 2 {
		return
	}
	fmt.Fprintf(out, "  %s\n", paint(tbl.Dim, fmt.Sprintf("all %d runs", len(rs))))
	fmt.Fprintf(out, "  %s   %s\n\n",
		paint(tbl.Accent, chart.Sparkline(wpms, width)),
		paint(tbl.Dim, fmt.Sprintf("avg %.0f", sum/float64(len(rs)))))
}

func writeRecent(out interface{ Write([]byte) (int, error) }, rs []history.Record, tbl theme.Table, limit int) {
	if limit <= 0 {
		limit = 10
	}
	start := len(rs) - limit
	if start < 0 {
		start = 0
	}
	recent := rs[start:]

	fmt.Fprintf(out, "  %s\n", paint(tbl.Dim,
		fmt.Sprintf("  %-12s %-8s %5s %5s %5s", "DATE", "MODE", "WPM", "ACC", "CONS")))
	for i := len(recent) - 1; i >= 0; i-- {
		r := recent[i]
		fmt.Fprintf(out, "  %s %s %s\n",
			paint(tbl.Dim, fmt.Sprintf("  %-12s %-8s", r.At.Format("Jan 2 15:04"), r.Key())),
			paint(tbl.Accent, fmt.Sprintf("%5.0f", r.WPM)),
			paint(tbl.Dim, fmt.Sprintf("%4.0f%% %4.0f%%", r.Accuracy, r.Consistency)))
	}
}
