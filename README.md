# thock

Monkeytype, straight in your terminal. It uses the same scoring formulas, so your numbers line up with monkeytype.com.

![Thock start screen: the ASCII thock banner above the time and word-list options, with the first three lines of words waiting to be typed](docs/screenshots/start.webp)

## Features

- Time, words and quotes modes, with a 1k or 5k word list
- Race people on your network, with everyone's cursor shown live in your own text
- wpm, raw, accuracy and consistency, calculated the way Monkeytype does it
- A stats screen with your run history, trend lines and personal bests
- 10 themes, plus `NO_COLOR` support
- Redraws at 120 FPS, and a keystroke takes about 4 µs to process

## Screenshots

| Mid-test | Results after a 30s test |
| :-: | :-: |
| <img src="docs/screenshots/typing.webp" alt="Thock mid-test with typed words in white, a mistyped letter in red and upcoming words dimmed, live wpm in the corner" width="400"> | <img src="docs/screenshots/results.webp" alt="Thock results after a 30-second test: 116 wpm, 99% accuracy, a raw-wpm-per-second bar chart with the one mistake marked" width="400"> |

## Stack

Go, Bubble Tea v2, Cobra.

## Install

```sh
go install github.com/dawsonxiong/thock@latest
```

or build it from a clone:

```sh
go build -o thock . && ./thock
```

You'll need Go 1.25+.

## Usage

```sh
thock                     # 30 second test on the 1k word list
thock --time 60           # 15, 30, 60 or 120 seconds
thock --words 50          # 10, 25, 50 or 100 words
thock --mode quotes       # a famous line from a film, book or show
thock --list 5k           # wider vocabulary
thock --theme gruvbox     # see `thock themes`
thock stats               # personal bests and recent runs
thock race                # find a race on your network, or host one
```

If you pass `--time` or `--words` you don't need `--mode` as well.

## Keys

| Key | |
|---|---|
| `esc` | restart with the same text |
| `enter` | new text (on the results screen) |
| `tab` | options (`enter` to apply and restart, `esc` to back out) |
| `ctrl+s` | stats (`←→` to filter, `esc` to go back) |
| `ctrl+r` | race (see [Racing](#racing)) |
| `ctrl+w` | delete the last word |
| `ctrl+c` | quit |

Typing on the results screen doesn't do anything, so an extra keystroke after your last word won't skip past your score.

## Stats

`ctrl+s` opens stats from an idle test or the results screen, and `esc` takes you back. It doesn't work in the middle of a test.

![Thock stats screen over ten runs: best and average wpm, a run-history chart, accuracy and consistency sparklines, and a per-mode table](docs/screenshots/stats.webp)

Each bar is one run, with a smoothed average underneath. Accuracy and consistency get their own sparklines, each labelled with its own min and max, and the table at the bottom has your bests and averages for each setup. `←→` switches between the setups you've played. Runs are only compared with the same setup, so a 30s run won't count toward your 60s bests.

`thock stats` prints the same numbers without opening the UI.

## Racing

Race anyone on the same network, each in their own terminal. One person hosts and the others join:

```sh
thock race host --words 25    # open a room: 10, 25, 50 or 100 words, or --quote short
thock race join velvet-orbit  # join by the room's code...
thock race join 192.168.1.20  # ...or its address
thock race                    # list the rooms on your network and pick one
```

Rooms announce themselves on the local network, so `thock race` usually finds them without typing anything. Every room also has a two-word code that spells its address, which still works on networks that block the announcements (guest Wi-Fi often does). `--name` sets the name other racers see, and thock remembers it.

![A race in progress: three lanes with pace trails above the text, and the other two racers' cursors shown as coloured blocks inside the words](docs/screenshots/race-typing.webp)

- **Everyone's cursor is in your text.** The other racers show up as coloured blocks on the letter they're typing, so you can see someone two words ahead of you without looking away from the words.
- **Lanes show pace, not just position.** Each lane's trail is shaded by how fast that racer was typing at that point in the text, so you can see where a lead came from.
- **The words stay hidden until zero.** The countdown shows only the shape of the text, and keys pressed early are ignored.
- **A time gap, not a character count.** The status line shows how long ago the racer ahead of you was where you are now, like the time gaps in a cycling race.
- **Replays.** Press `r` on the results screen to watch the whole race again at 2×, with every cursor moving through the text.

| Lobby | Results |
| :-: | :-: |
| <img src="docs/screenshots/race-lobby.webp" alt="The race lobby: the thock banner, the race setup, three racers and the command to join" width="400"> | <img src="docs/screenshots/race-podium.webp" alt="Race results: places, wpm, accuracy and finishing times, each racer's pace on a shared clock, and a running tally of wins" width="400"> |

The host picks the setup with `←→` and starts each round with `enter`. Anyone can press `esc` to give up a round or leave the room. A room holds up to eight racers, and anyone who arrives mid-round races in the next one.

Everyone's finishing time is measured on their own machine from a shared start, so network lag doesn't decide who wins. The host scores every run from its keystrokes with the same formulas as a solo test. A race counts toward your personal bests like any other words or quote test.

`thock race host --headless` runs a room with no screen, for a spare machine to hold. The first person to join gets the host controls.

## Scoring

Same formulas as Monkeytype:

- **wpm**: correct characters, plus one space for each word typed perfectly, divided by five, per minute
- **raw**: the same thing but counting everything you typed
- **accuracy**: based on every keystroke, so a mistake still counts even if you fixed it
- **consistency**: Monkeytype's `kogasa` function applied to the coefficient of variation of your per-second raw speed (population standard deviation)

If you hit space early and skip part of a word, the skipped characters count against your speed but not your accuracy.

## Performance

The benchmarks are in `internal/ui/bench_test.go`:

```
go test ./internal/ui -bench 'Keystroke|View' -benchmem
```

On an M4 (Go 1.27, 120x40 terminal, words mode, 1k list, run on 2026-09-22):

| Benchmark | Time | Allocations |
|---|---|---|
| Keystroke (`Update` then `View`) | ~4.1 µs | 43 |
| View only | ~1.5 µs | 36 |

A frame at 120 FPS is 8.3 ms, so a keystroke uses about 1/2000th of it.

## Themes

`mono` (default), `amber`, `neon`, `terminal`, `catppuccin`, `gruvbox`, `nord`, `dracula`, `tokyonight` and `rosepine`. Run `thock themes` to preview them.

`terminal` only uses ANSI colours 0–15, so it matches whatever palette your terminal has. With `NO_COLOR` or `--no-color`, thock uses underline and dim text instead of colour.

## Files

| Path | |
|---|---|
| `~/.config/thock/config.toml` | theme, last-used test settings and your race name |
| `~/.local/share/thock/results.jsonl` | one JSON object per finished test |

Both respect `XDG_CONFIG_HOME` and `XDG_DATA_HOME`. Results are plain JSONL and stay on your machine. Racing uses TCP on ports 47300–47307 and multicast on `239.255.77.77:47777`, and only talks to machines on your network.

## Not affiliated with Monkeytype

thock uses Monkeytype's scoring formulas but has no connection to Monkeytype, and it doesn't talk to monkeytype.com.

## Licence

MIT
