package race

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dawsonxiong/thock/internal/typing"
)

var text = []string{"the", "quick", "fox"}

// typeOut builds the keystrokes of someone typing words perfectly at a steady
// gap between keys, starting one gap after zero.
func typeOut(words []string, gap time.Duration) Keys {
	var keys Keys
	at := time.Duration(0)
	for i, w := range words {
		for _, r := range w {
			at += gap
			keys = append(keys, Key{At: at, R: r, Kind: typing.KeyChar})
		}
		if i < len(words)-1 {
			at += gap
			keys = append(keys, Key{At: at, R: ' ', Kind: typing.KeySpace})
		}
	}
	return keys
}

func TestKeysRoundTripCompactly(t *testing.T) {
	in := Keys{
		{At: 120 * time.Millisecond, R: 't', Kind: typing.KeyChar},
		{At: 250 * time.Millisecond, Kind: typing.KeyBackspace},
		{At: 300 * time.Millisecond, R: 'é', Kind: typing.KeyChar},
	}
	b, err := json.Marshal(Msg{T: MsgFinish, Keys: in})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"keys":[[120,116,0],[250,0,2],[300,233,0]]`) {
		t.Fatalf("unexpected encoding: %s", b)
	}
	var out Msg
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Keys) != 3 || out.Keys[2] != in[2] || out.Keys[1] != in[1] {
		t.Fatalf("round trip lost keys: %+v", out.Keys)
	}
}

func TestKeysRejectsGarbage(t *testing.T) {
	for _, bad := range []string{`[[1,2]]`, `[[-1,97,0]]`, `[[1,97,9]]`, `[[1,2000000,0]]`, `{"x":1}`} {
		var k Keys
		if err := json.Unmarshal([]byte(bad), &k); err == nil {
			t.Errorf("%s: accepted", bad)
		}
	}
}

func TestCleanKeysDropsWhatTypingCannotProduce(t *testing.T) {
	in := Keys{
		{R: 'a', Kind: typing.KeyChar},
		{R: '\x1b', Kind: typing.KeyChar},
		{R: '\n', Kind: typing.KeyChar},
		{R: ' ', Kind: typing.KeySpace},
		{Kind: typing.KeyBackspace},
		{R: 'é', Kind: typing.KeyChar},
	}
	out := in.Clean()
	if len(out) != 4 || out[0].R != 'a' || out[1].Kind != typing.KeySpace || out[3].R != 'é' {
		t.Errorf("cleaned to %+v", out)
	}
}

func TestScoreReplaysKeysToTheSameResult(t *testing.T) {
	keys := typeOut(text, 100*time.Millisecond)
	res, done := Score(text, keys)
	if !done {
		t.Fatal("perfect keys should finish the text")
	}
	if res.Accuracy != 100 {
		t.Errorf("accuracy %.1f, want 100", res.Accuracy)
	}
	// 11 correct characters plus 2 spaces in 1.3s.
	want := float64(11+2) / 5 / (1.3 / 60)
	if diff := res.WPM - want; diff > 0.01 || diff < -0.01 {
		t.Errorf("wpm %.2f, want %.2f", res.WPM, want)
	}

	if _, done := Score(text, keys[:len(keys)-1]); done {
		t.Error("stopping one letter short should not finish")
	}
}

func TestPositionCountsOnlyTheMatchedPart(t *testing.T) {
	e := typing.New(text)
	if _, _, d := Position(e); d != 0 {
		t.Fatalf("fresh text covered %.2f", d)
	}
	for _, r := range "the" {
		e.TypeRune(r, 0)
	}
	e.Space(0)
	for _, r := range "quickkkk" { // overtyped
		e.TypeRune(r, 0)
	}
	w, c, d := Position(e)
	if w != 1 || c != 5 {
		t.Errorf("at word %d char %d, want 1, 5", w, c)
	}
	// "the " + "quick" of "the quick fox" (13 characters with spaces).
	if want := 9.0 / 13; d < want-0.001 || d > want+0.001 {
		t.Errorf("covered %.3f, want %.3f", d, want)
	}
}

func TestClockKeepsTheTightestSample(t *testing.T) {
	var c Clock
	// Host is 5s ahead. A 40ms round trip, then a 2ms one.
	c.Observe(1000*time.Millisecond, 6020*time.Millisecond, 1040*time.Millisecond)
	c.Observe(2000*time.Millisecond, 7001*time.Millisecond, 2002*time.Millisecond)
	// A slow, misleading sample must not replace the good one.
	c.Observe(3000*time.Millisecond, 8400*time.Millisecond, 3500*time.Millisecond)
	if got := c.Local(10 * time.Second); got != 5*time.Second {
		t.Errorf("host 10s maps to local %v, want 5s", got)
	}
	if c.RTT() != 2*time.Millisecond {
		t.Errorf("rtt %v", c.RTT())
	}
}

func TestReachedAndTrail(t *testing.T) {
	tr := []Point{{At: 1 * time.Second, Done: 0.1, WPM: 40}, {At: 2 * time.Second, Done: 0.5, WPM: 80}, {At: 3 * time.Second, Done: 0.9, WPM: 90}}
	if at, ok := Reached(tr, 0.4); !ok || at != 2*time.Second {
		t.Errorf("reached 0.4 at %v %v", at, ok)
	}
	if _, ok := Reached(tr, 0.95); ok {
		t.Error("never got to 0.95")
	}
	trail := Trail(tr, 11)
	// Columns 1, 5 and 9 hold samples; the ones between carry the pace forward,
	// and the one before the first sample takes its pace.
	if len(trail) != 10 || trail[0] != 40 || trail[1] != 40 || trail[3] != 40 || trail[5] != 80 || trail[9] != 90 {
		t.Errorf("trail %v", trail)
	}
}

func TestCleanNameStripsEscapes(t *testing.T) {
	for in, want := range map[string]string{
		"sam":                        "sam",
		"\x1b[2Jevil\x1b]0;x\x07":    "[2Jevil]0;x",
		"   ":                        "racer",
		"a very long name indeed ok": "a very long",
		"tab\tand\nnewline":          "tab and newl",
	} {
		if got := CleanName(in); got != want {
			t.Errorf("CleanName(%q) = %q, want %q", in, got, want)
		}
	}
	if got := CleanCode("your-knowledge"); got != "your-knowledge" {
		t.Errorf("a long code was cut to %q", got)
	}
	if got := CleanCode("Velvet-\x1b[31mOrbit"); got != "velvet-morbit" {
		t.Errorf("CleanCode kept %q", got)
	}
	if err := CheckWords([]string{"fine", "b\x1bad"}); err == nil {
		t.Error("a word with an escape must be refused")
	}
	if err := CheckWords([]string{"two words"}); err == nil {
		t.Error("a word with a space must be refused")
	}
}

func TestOrdinal(t *testing.T) {
	for n, want := range map[int]string{1: "1st", 2: "2nd", 3: "3rd", 4: "4th", 11: "11th", 12: "12th", 21: "21st", 113: "113th"} {
		if got := Ordinal(n); got != want {
			t.Errorf("Ordinal(%d) = %q", n, got)
		}
	}
}

// A whole round on a fake clock: three racers, one quick, one slower, one who
// gives up, plus a late arrival who watches.
func TestRoomRunsARound(t *testing.T) {
	setup := Setup{Mode: ModeWords, Words: 10, List: "1k"}
	r := NewRoom("dawson", "velvet-orbit", setup)
	host, _ := r.Join("dawson", true)
	maya, _ := r.Join("maya", false)
	sam, _ := r.Join("sam", false)

	if _, err := r.Begin(maya, text, "", 0); err != ErrNotHost {
		t.Fatalf("a guest started a round: %v", err)
	}
	now := 10 * time.Second
	st, err := r.Begin(host, text, "", now)
	if err != nil {
		t.Fatal(err)
	}
	if st.Round != 1 || Dur(st.GoAt) != now+Countdown {
		t.Fatalf("start %+v", st)
	}
	late, _ := r.Join("late", false)
	if r.Tick(now + time.Second); r.Phase() != PhaseCountdown {
		t.Fatalf("opened early: %s", r.Phase())
	}
	goAt := now + Countdown
	r.Tick(goAt)
	if r.Phase() != PhaseRacing {
		t.Fatalf("phase %s after the countdown", r.Phase())
	}
	if err := r.Report(late, 1, Progress{Done: 0.5}); err != ErrNotRacing {
		t.Errorf("a watcher reported progress: %v", err)
	}

	fast := typeOut(text, 80*time.Millisecond)
	slow := typeOut(text, 150*time.Millisecond)
	if err := r.Finish(maya, 1, fast[:5], goAt+2*time.Second); err != ErrUnfinished {
		t.Errorf("half a text finished the race: %v", err)
	}
	if err := r.Finish(maya, 1, fast, goAt+2*time.Second); err != nil {
		t.Fatal(err)
	}
	// A time nobody's clock could have produced is refused.
	wild := typeOut(text, time.Second)
	if err := r.Finish(host, 1, wild, goAt+2*time.Second); err != ErrUnfinished {
		t.Errorf("a finish from the future was accepted: %v", err)
	}
	if r.Tick(goAt + 2*time.Second) {
		t.Fatal("closed with racers still out")
	}
	if err := r.Finish(host, 1, slow, goAt+3*time.Second); err != nil {
		t.Fatal(err)
	}
	r.Forfeit(sam, 1, fast[:4])
	if !r.Tick(goAt + 3*time.Second) {
		t.Fatal("did not close once everyone was in or out")
	}

	res := r.Results()
	if len(res.Standings) != 3 {
		t.Fatalf("%d standings, want 3 (the late arrival did not race)", len(res.Standings))
	}
	if a, b, c := res.Standings[0], res.Standings[1], res.Standings[2]; a.ID != maya || a.Place != 1 ||
		b.ID != host || b.Place != 2 || c.ID != sam || c.Place != 0 {
		t.Fatalf("standings %+v", res.Standings)
	}
	if res.Standings[2].Done <= 0 {
		t.Error("a forfeit should keep how far they got")
	}
	if len(res.Keys) != 3 {
		t.Errorf("replay keys for %d racers, want 3", len(res.Keys))
	}
	for _, p := range r.State().Players {
		if p.ID == maya && p.Wins != 1 {
			t.Errorf("maya has %d wins", p.Wins)
		}
	}

	// The next round includes the late arrival.
	if _, err := r.Begin(host, text, "", goAt+10*time.Second); err != nil {
		t.Fatal(err)
	}
	for _, p := range r.State().Players {
		if !p.Racing || p.Finished || p.Done != 0 {
			t.Errorf("%s not reset for the new round: %+v", p.Name, p)
		}
	}
}

func TestRoomClosesAfterGrace(t *testing.T) {
	r := NewRoom("h", "", Setup{Mode: ModeWords, Words: 10, List: "1k"})
	host, _ := r.Join("h", true)
	r.Join("afk", false)
	r.Begin(host, text, "", 0)
	r.Tick(Countdown)
	keys := typeOut(text, 100*time.Millisecond)
	fin := Countdown + 2*time.Second
	r.Finish(host, 1, keys, fin)
	if r.Tick(fin + minGrace - time.Millisecond) {
		t.Fatal("closed before the grace ran out")
	}
	if !r.Tick(fin + minGrace) {
		t.Fatal("waited forever for someone who never typed")
	}
	if s := r.Results().Standings; s[1].Place != 0 {
		t.Errorf("the absent racer placed: %+v", s[1])
	}
}

func TestRoomNamesAndLimits(t *testing.T) {
	r := NewRoom("h", "", Setup{Mode: ModeWords, Words: 10, List: "1k"})
	for i := 0; i < MaxPlayers; i++ {
		if _, err := r.Join("sam", i == 0); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.Join("one more", false); err != ErrFull {
		t.Errorf("joined a full room: %v", err)
	}
	names := map[string]bool{}
	for _, p := range r.State().Players {
		if names[p.Name] {
			t.Errorf("two players called %q", p.Name)
		}
		names[p.Name] = true
	}
	if err := r.SetSetup(1, Setup{Mode: ModeWords, Words: 7, List: "1k"}); err != ErrBadSetup {
		t.Errorf("odd setup accepted: %v", err)
	}
	if err := r.SetSetup(2, Setup{Mode: ModeQuotes, Length: "short"}); err != ErrNotHost {
		t.Errorf("a guest changed the setup: %v", err)
	}
	if err := r.SetSetup(1, Setup{Mode: ModeQuotes, Length: "short"}); err != nil {
		t.Error(err)
	}
}

func TestLeavingMidRoundKeepsTheLaneUntilTheEnd(t *testing.T) {
	r := NewRoom("h", "", Setup{Mode: ModeWords, Words: 10, List: "1k"})
	host, _ := r.Join("h", true)
	gone, _ := r.Join("gone", false)
	r.Begin(host, text, "", 0)
	r.Tick(Countdown)
	r.Leave(gone)
	if n := len(r.State().Players); n != 2 {
		t.Fatalf("%d players mid-round, want the leaver kept as out", n)
	}
	r.Finish(host, 1, typeOut(text, 100*time.Millisecond), Countdown+2*time.Second)
	if !r.Tick(Countdown + 2*time.Second) {
		t.Fatal("round did not close with the leaver out")
	}
	if n := len(r.Results().Standings); n != 2 {
		t.Errorf("%d standings, want the leaver listed", n)
	}
	if n := len(r.State().Players); n != 1 {
		t.Errorf("%d players after the round, want the leaver gone", n)
	}
}

func TestHeadlessRoomPromotes(t *testing.T) {
	r := NewRoom("spare", "", Setup{Mode: ModeWords, Words: 10, List: "1k"})
	a, _ := r.Join("a", true)
	b, _ := r.Join("b", false)
	r.Leave(a)
	r.Promote()
	if !r.IsHost(b) {
		t.Error("the controls were not handed on")
	}
}
