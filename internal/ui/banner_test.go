package ui

import (
	"testing"

	"github.com/mattn/go-runewidth"
)

// The wordmark is drawn as fixed rows against a left margin, so a row of the
// wrong width would shear it and a double-width rune would push it off centre.
func TestBannerRowsAreEqualWidth(t *testing.T) {
	for i, line := range banner {
		if got := runewidth.StringWidth(line); got != bannerWidth {
			t.Errorf("banner row %d is %d cells, want %d", i, got, bannerWidth)
		}
		for _, r := range line {
			if runewidth.RuneWidth(r) != 1 {
				t.Errorf("banner row %d contains non-single-width rune %q", i, r)
			}
		}
	}
}

// A viewport too narrow or too short for the wordmark must drop it rather than
// wrap it, since a wrapped banner would corrupt every row below it.
func TestBannerHiddenWhenItDoesNotFit(t *testing.T) {
	for _, tc := range []struct {
		name       string
		w, h       int
		wantBanner bool
	}{
		{"roomy", 100, 30, true},
		{"too narrow", bannerWidth + 6 - 1, 30, false},
		{"too short", 100, 17, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModel(t, tc.w, tc.h)
			frame := strip(m.View().Content)
			// The bowl of the "o" appears only in the wordmark, never in the
			// interface chrome.
			got := contains(frame, `/ _ \`)
			if got != tc.wantBanner {
				t.Errorf("banner present = %v, want %v at %dx%d", got, tc.wantBanner, tc.w, tc.h)
			}
		})
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
