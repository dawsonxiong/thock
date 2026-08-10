package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dawsonxiong/thock/internal/theme"
)

func themesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "themes",
		Short: "List the available colour themes",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return listThemes(cmd) },
	}
}

// listThemes prints each theme with a live sample, so the choice is made by
// looking at the colours rather than by reading their names.
func listThemes(cmd *cobra.Command) error {
	colour := colourEnabled(false)
	out := cmd.OutOrStdout()
	fmt.Fprintln(out)

	for _, name := range theme.Names() {
		t, _ := theme.Get(name)
		tbl := theme.Resolve(t, colour)

		// A miniature of the real thing: correct, wrong and pending text plus
		// the accent. Padding is measured on the plain words, because the
		// escape sequences would otherwise be counted as visible width.
		const plain = "typed wrng pending 104"
		var sample strings.Builder
		sample.WriteString(paint(tbl.Char[1], "typed"))
		sample.WriteByte(' ')
		sample.WriteString(paint(tbl.Char[2], "wrng"))
		sample.WriteByte(' ')
		sample.WriteString(paint(tbl.Char[0], "pending"))
		sample.WriteByte(' ')
		sample.WriteString(paint(tbl.Accent, "104"))
		sample.WriteString(strings.Repeat(" ", 24-len(plain)))

		marker := "  "
		if name == theme.Default {
			marker = paint(tbl.Accent, "❯ ")
		}
		fmt.Fprintf(out, "%s%-12s %s%s\n", marker, name, sample.String(), paint(tbl.Dim, t.Desc))
	}
	fmt.Fprintln(out)
	return nil
}

func paint(prefix, s string) string {
	if prefix == "" {
		return s
	}
	return prefix + s + "\x1b[0m"
}
