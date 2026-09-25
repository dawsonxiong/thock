// Package history appends finished tests to a JSONL file and reads them back.
// A flat append-only file keeps the tool dependency-free and leaves the data
// greppable, which matters more here than query power.
package history

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// Record is one finished test as stored on disk.
type Record struct {
	At          time.Time `json:"at"`
	Mode        string    `json:"mode"`     // time | words | quotes
	Duration    int       `json:"duration"` // seconds
	Words       int       `json:"words,omitempty"`
	List        string    `json:"list,omitempty"` // time and words modes
	Source      string    `json:"source,omitempty"`
	WPM         float64   `json:"wpm"`
	Raw         float64   `json:"raw"`
	Accuracy    float64   `json:"accuracy"`
	Consistency float64   `json:"consistency"`
	Chars       [4]int    `json:"chars"` // correct, incorrect, extra, missed
	Race        *Race     `json:"race,omitempty"`
}

// Race marks a record typed in a race. A race is a real words or quote test,
// so it counts toward the same bests; this only says where it happened.
type Race struct {
	Place int `json:"place"` // 0 when the racer did not finish
	Field int `json:"field"` // how many raced
}

// Key groups records that are comparable for a personal best: a 30 second run
// is only a best against other 30 second runs, and a 25 word run against other
// 25 word runs.
func (r Record) Key() string {
	switch {
	case r.Mode == "quotes":
		return "quote"
	// Records written before word-count mode existed used "words" to mean a
	// timed test and carry no word count.
	case r.Mode == "words" && r.Words > 0:
		return itoa(r.Words) + "w"
	default:
		return itoa(r.Duration) + "s"
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// Dir is the directory holding results.jsonl, honouring XDG_DATA_HOME.
func Dir() (string, error) {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "thock"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "thock"), nil
}

// Path is the full path to results.jsonl.
func Path() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "results.jsonl"), nil
}

// Append adds one record to the log.
func Append(r Record) error {
	d, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(d, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(d, "results.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		return err
	}
	return nil
}

// Load reads every record. Unparseable lines are skipped rather than fatal, so
// a truncated write can never lock the user out of their own history.
func Load() ([]Record, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []Record
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var r Record
		if json.Unmarshal(line, &r) != nil {
			continue
		}
		out = append(out, r)
	}
	return out, sc.Err()
}

// Best returns the highest WPM recorded for each comparable group.
func Best(rs []Record) map[string]Record {
	out := make(map[string]Record)
	for _, r := range rs {
		k := r.Key()
		if cur, ok := out[k]; !ok || r.WPM > cur.WPM {
			out[k] = r
		}
	}
	return out
}

// IsPB reports whether r beats every earlier record in its group.
func IsPB(rs []Record, r Record) bool {
	for _, o := range rs {
		if o.Key() == r.Key() && o.WPM >= r.WPM {
			return false
		}
	}
	return true
}
