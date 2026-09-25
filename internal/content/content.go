// Package content supplies the text to type: random words drawn from an
// embedded frequency list, or a curated quote. Everything is compiled into the
// binary, so thock never touches the network.
package content

import (
	"embed"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"sync"
)

//go:embed data/english1k.txt data/english5k.txt data/quotes.json
var files embed.FS

// List identifies a word list.
type List string

const (
	List1k List = "1k"
	List5k List = "5k"
)

// Length is a quote size bucket.
type Length string

const (
	LengthAny    Length = "any"
	LengthShort  Length = "short"  // up to 80 characters
	LengthMedium Length = "medium" // up to 160
	LengthLong   Length = "long"   // longer than 160
)

// Quote is a single attributed passage.
type Quote struct {
	Text   string `json:"text"`
	Source string `json:"source"`
	Author string `json:"author,omitempty"`
	Year   int    `json:"year,omitempty"`
	Medium string `json:"medium"`
}

// Attribution renders the credit line shown under a result.
func (q Quote) Attribution() string {
	var b strings.Builder
	b.WriteString(q.Source)
	if q.Author != "" {
		b.WriteString(", ")
		b.WriteString(q.Author)
	}
	if q.Year > 0 {
		b.WriteString(" (")
		b.WriteString(strconv.Itoa(q.Year))
		b.WriteString(")")
	}
	return b.String()
}

// Bucket classifies a quote by length.
func (q Quote) Bucket() Length {
	switch n := len([]rune(q.Text)); {
	case n <= 80:
		return LengthShort
	case n <= 160:
		return LengthMedium
	default:
		return LengthLong
	}
}

var (
	once   sync.Once
	words  map[List][]string
	quotes []Quote
	loadEr error
)

func load() {
	once.Do(func() {
		words = make(map[List][]string, 2)
		for _, l := range []struct {
			list List
			path string
		}{{List1k, "data/english1k.txt"}, {List5k, "data/english5k.txt"}} {
			b, err := files.ReadFile(l.path)
			if err != nil {
				loadEr = fmt.Errorf("read %s: %w", l.path, err)
				return
			}
			words[l.list] = strings.Fields(string(b))
		}
		b, err := files.ReadFile("data/quotes.json")
		if err != nil {
			loadEr = fmt.Errorf("read quotes: %w", err)
			return
		}
		if err := json.Unmarshal(b, &quotes); err != nil {
			loadEr = fmt.Errorf("parse quotes: %w", err)
			return
		}
	})
}

// WordCountFor sizes a word buffer for a timed test, allowing for a typist far
// faster than anyone is likely to be so the words never run out mid-test.
func WordCountFor(seconds int) int {
	const wordsPerSecond = 5 // 300 wpm
	return seconds*wordsPerSecond + 50
}

// Words returns n random words from a list, never repeating a word twice in
// a row.
func Words(list List, n int) ([]string, error) {
	load()
	if loadEr != nil {
		return nil, loadEr
	}
	src, ok := words[list]
	if !ok {
		return nil, fmt.Errorf("unknown word list %q", list)
	}
	out := make([]string, 0, n)
	prev := ""
	for len(out) < n {
		w := src[rand.IntN(len(src))]
		if w == prev {
			continue
		}
		out = append(out, w)
		prev = w
	}
	return out, nil
}

// RandomQuote returns a quote from the given bucket.
func RandomQuote(length Length) (Quote, error) {
	load()
	if loadEr != nil {
		return Quote{}, loadEr
	}
	pool := quotes
	if length != LengthAny && length != "" {
		pool = pool[:0:0]
		for _, q := range quotes {
			if q.Bucket() == length {
				pool = append(pool, q)
			}
		}
	}
	if len(pool) == 0 {
		return Quote{}, fmt.Errorf("no quotes in bucket %q", length)
	}
	return pool[rand.IntN(len(pool))], nil
}

// QuoteWords splits a quote into the words the engine will test against.
func QuoteWords(q Quote) []string { return strings.Fields(q.Text) }

// Vocabulary returns a word list in its stored order. Room codes are spelled
// with it, so the order is part of the race protocol and must not change.
func Vocabulary(list List) ([]string, error) {
	load()
	if loadEr != nil {
		return nil, loadEr
	}
	src, ok := words[list]
	if !ok {
		return nil, fmt.Errorf("unknown word list %q", list)
	}
	return append([]string(nil), src...), nil
}
