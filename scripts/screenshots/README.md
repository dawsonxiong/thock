# Screenshots

Captures the portfolio screenshots from real thock runs, in the Tokyo Night theme.

```sh
cd scripts/screenshots
pnpm install
pnpm capture
```

It builds the working tree's thock, runs it in a pseudo-terminal with a scripted typist, and draws the output with xterm.js in headless Chrome. Every number in the shots comes from a real test. History and config go in a temporary directory, so your own stats are untouched.

About five minutes later, `out/` holds a 2x PNG and a 1600-wide WebP for each screen:

| Name      | Screen                                      |
| --------- | ------------------------------------------- |
| `start`   | Banner, config bar and words before typing  |
| `typing`  | Mid-test, with one deliberate typo          |
| `results` | Results of a 30s test                       |
| `stats`   | Stats over ten runs (30s, 25 words, quotes) |

`pnpm capture:race` takes the race shots the same way, from a real race between three thock processes on the same machine. maya hosts, sam joins by code, and dawson, the terminal in the shots, finds the room in the list and joins it. Each has its own typist at its own speed. It takes about two minutes:

| Name             | Screen                                              |
| ---------------- | --------------------------------------------------- |
| `race-browse`    | The room list, with maya's room found               |
| `race-lobby`     | Three racers waiting for the host                   |
| `race-countdown` | The countdown, text still hidden                    |
| `race-typing`    | Mid-race, with maya's and sam's cursors in the text |
| `race-finished`  | Over the line, waiting on sam                       |
| `race-podium`    | Round one's results                                 |
| `race-replay`    | The replay, part way through                        |
| `race-podium-2`  | Round two, with the tally of wins                   |

`--out <dir>` writes somewhere else. To update the portfolio site (browsers and Next's image cache key on the URL, so a replacement under an old name can stay stale; pick a new suffix and update `src/content/portfolio.ts` when the look changes):

```sh
for f in start typing results stats; do cp out/$f.webp ~/Developer/personal-website/public/portfolio/thock/$f-tokyonight.webp; done
```

The main README shows the same shots from `docs/screenshots/`, under their plain names:

```sh
cp out/{start,typing,results,stats}.webp ../../docs/screenshots/
cp out/race-{typing,lobby,podium}.webp ../../docs/screenshots/
```

Needs Go, Node 22 and Google Chrome. Elsewhere, `CHROME=/path/to/chromium` uses another Chromium build. Chromium on Linux measures JetBrains Mono taller than Chrome on macOS; `LINE_HEIGHT=1.125` makes a Linux race capture match the rows of the published shots. xterm stays on 5.x because the canvas renderer, which draws the chart's block characters without gaps, was dropped in 6.
