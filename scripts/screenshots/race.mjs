// Captures the race screenshots from a real race between three thock processes on this machine.
//
// maya hosts a room, sam joins it by code, and dawson (the terminal in the shots) finds it in the
// room list and joins. Each runs in its own pseudo-terminal with its own history and config, and
// each has a scripted typist that reads the words off its own screen, at its own speed. Only
// dawson's terminal is photographed; the other two are drawn off-screen so their typists can read
// them. Every lane, ghost caret and result comes from the race actually run.
//
//   pnpm capture:race             # writes out/race-<name>.png (2x) and .webp (1600 wide)
//   pnpm capture:race --out <dir>
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import { createRequire } from "node:module";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { parseArgs } from "node:util";
import pty from "node-pty";
import { chromium } from "playwright-core";
import sharp from "sharp";

const HERE = path.dirname(fileURLToPath(import.meta.url));
const REPO = path.resolve(HERE, "../..");
const { values: args } = parseArgs({ options: { out: { type: "string", default: path.join(HERE, "out") } } });
const OUT = path.resolve(args.out);

// The same window as capture.mjs, so the race shots sit alongside the others.
const COLS = 96;
const ROWS = 24;
const THEME = "tokyonight";
const WEBP_WIDTH = 1600;
const WINDOW = { background: "#1a1b26", foreground: "#c0caf5", cursor: "#7aa2f7" };
// capture.mjs uses a line height of 1.25 on macOS. Chromium on Linux measures JetBrains Mono's
// cell taller, so LINE_HEIGHT lets a Linux run match the rows of the shots already published.
const LINE_HEIGHT = Number(process.env.LINE_HEIGHT ?? 1.25);

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const log = (...a) => console.log(new Date().toISOString().slice(11, 19), ...a);

const TMP = fs.mkdtempSync(path.join(os.tmpdir(), "thock-race-"));
const BIN = path.join(TMP, "thock");
log("building thock");
execFileSync("go", ["build", "-o", BIN, "."], { cwd: REPO, stdio: "inherit" });
fs.mkdirSync(OUT, { recursive: true });

const ptyDir = path.dirname(createRequire(import.meta.url).resolve("node-pty/package.json"));
const helper = path.join(ptyDir, "prebuilds", `${process.platform}-${process.arch}`, "spawn-helper");
if (fs.existsSync(helper)) fs.chmodSync(helper, 0o755);

const PLAYERS = ["dawson", "maya", "sam"];
const mod = (p) => fs.readFileSync(path.join(HERE, "node_modules", p), "utf8");
const browser = await chromium.launch(
  process.env.CHROME ? { executablePath: process.env.CHROME, headless: true } : { channel: "chrome", headless: true },
);
const page = await (await browser.newContext({ viewport: { width: 1180, height: 560 }, deviceScaleFactor: 2 })).newPage();
// dawson's window is the one on screen; the others sit below the fold, where the screenshot of
// .win never reaches.
await page.setContent(`<!doctype html><html><head><style>${mod("@xterm/xterm/css/xterm.css")}
  html,body{margin:0;background:#0b0c14}
  .win{position:absolute;left:24px;top:24px;border-radius:12px;background:${WINDOW.background};padding:34px 26px 22px;box-shadow:0 20px 60px #0008}
  .off{position:absolute;left:24px;top:2000px}
  .dots{position:absolute;left:16px;top:12px;display:flex;gap:8px}.dots i{width:12px;height:12px;border-radius:50%;display:block}
  .xterm .xterm-viewport{background:${WINDOW.background} !important}
</style></head><body>
<div class="win"><div class="dots"><i style="background:#ff5f57"></i><i style="background:#febc2e"></i><i style="background:#28c840"></i></div><div id="t-dawson"></div></div>
<div class="off"><div id="t-maya"></div><div id="t-sam"></div></div>
<script>${mod("@xterm/xterm/lib/xterm.js")}</script>
<script>${mod("@xterm/addon-canvas/lib/addon-canvas.js")}</script>
<script>
  window.terms = {};
  for (const name of ${JSON.stringify(PLAYERS)}) {
    const term = new Terminal({ cols: ${COLS}, rows: ${ROWS}, fontFamily: '"JetBrains Mono", Menlo, monospace', fontSize: 15, lineHeight: ${LINE_HEIGHT}, customGlyphs: true, cursorBlink: false, cursorStyle: "bar", theme: ${JSON.stringify(WINDOW)} });
    term.open(document.getElementById("t-" + name));
    if (name === "dawson") {
      term.loadAddon(new CanvasAddon.CanvasAddon());
      term.focus();
    }
    window.terms[name] = term;
  }
  window.lineAt = (n, y) => window.terms[n].buffer.active.getLine(y)?.translateToString(true) ?? "";
  window.cursor = (n) => ({ x: window.terms[n].buffer.active.cursorX, y: window.terms[n].buffer.active.cursorY });
</script></body></html>`);
await page.waitForFunction(() => !!window.terms?.dawson);
await sleep(300);
await page.evaluate(() => {
  window.terms.dawson.options.fontSize = 16;
  window.terms.dawson.options.fontSize = 15;
});
await page.waitForFunction(() => {
  const c = document.querySelector("#t-dawson .xterm-text-layer");
  return c && c.width === 2 * parseInt(c.style.width);
});

// One racer: a pty running thock, feeding its own off-page terminal.
class Racer {
  constructor(name) {
    this.name = name;
    this.pending = "";
    this.proc = null;
  }
  start(args) {
    const home = path.join(TMP, this.name);
    this.pending = "";
    this.proc = pty.spawn(BIN, [...args, "--theme", THEME, "--name", this.name], {
      name: "xterm-256color",
      cols: COLS,
      rows: ROWS,
      cwd: TMP,
      env: {
        ...process.env,
        TERM: "xterm-256color",
        COLORTERM: "truecolor",
        LANG: "en_US.UTF-8",
        XDG_DATA_HOME: path.join(home, "data"),
        XDG_CONFIG_HOME: path.join(home, "config"),
      },
    });
    this.proc.onData((d) => {
      this.pending += d;
    });
  }
  stop() {
    this.proc?.kill();
    this.proc = null;
  }
  key(s) {
    this.proc.write(s);
  }
  async flush() {
    if (!this.pending) return;
    const d = this.pending;
    this.pending = "";
    await page.evaluate(([n, s]) => new Promise((r) => window.terms[n].write(s, r)), [this.name, d]);
  }
  async screen() {
    await this.flush();
    return (await page.evaluate(([n, rows]) => Array.from({ length: rows }, (_, y) => window.lineAt(n, y)), [this.name, ROWS])).join("\n");
  }
  async waitFor(re, timeout = 40_000) {
    const end = Date.now() + timeout;
    while (Date.now() < end) {
      if (re.test(await this.screen())) return (await this.screen()).match(re);
      await sleep(100);
    }
    throw new Error(`${this.name}: timed out waiting for ${re}\n${await this.screen()}`);
  }
  // Types whatever sits at the cursor, a word at a time, re-reading the screen after each word,
  // until `until` matches. `pace` is the gap between keys in ms; `typo` fumbles one early word.
  async race({ pace, jitter = 30, until, typo = false }) {
    let words = 0;
    let typoDone = !typo;
    for (;;) {
      await this.flush();
      if (until.test(await this.screen())) return;
      const { x, y } = await page.evaluate((n) => window.cursor(n), this.name);
      const line = await page.evaluate(([n, y]) => window.lineAt(n, y), [this.name, y]);
      let word = line.slice(x).trim().split(/\s+/)[0];
      if (!word) {
        word = (await page.evaluate(([n, y]) => window.lineAt(n, y + 1), [this.name, y])).trim().split(/\s+/)[0];
        if (!word) {
          await sleep(100);
          continue;
        }
      }
      const text = !typoDone && words >= 3 && word.length > 3 ? ((typoDone = true), word.slice(0, 2) + "x" + word.slice(3)) : word;
      for (const ch of text + " ") {
        this.key(ch);
        await sleep(pace + Math.random() * jitter);
      }
      words++;
    }
  }
}

const racers = Object.fromEntries(PLAYERS.map((n) => [n, new Racer(n)]));
const { dawson, maya, sam } = racers;

const shots = [];
async function shot(name) {
  await dawson.flush();
  await sleep(120);
  await dawson.flush();
  const png = path.join(OUT, `${name}.png`);
  await page.locator(".win").screenshot({ path: png });
  await sharp(png).resize({ width: WEBP_WIDTH }).webp({ quality: 90 }).toFile(path.join(OUT, `${name}.webp`));
  shots.push(name);
  log("shot", name);
}

// Keeps a racer's terminal current while the others are busy, so its typist and ours read fresh
// screens.
function pump(r) {
  let on = true;
  (async () => {
    while (on) {
      await r.flush();
      await sleep(50);
    }
  })();
  return () => (on = false);
}

const PODIUM = /pace/;

try {
  // maya opens a room, and sam joins it by the code on her lobby screen.
  maya.start(["race", "host", "--words", "25", "--list", "1k"]);
  const [, code] = await maya.waitFor(/thock race join ([a-z]+-[a-z]+)/);
  log("room code", code);
  sam.start(["race", "join", code]);
  await maya.waitFor(/2 racers/);

  // dawson looks for rooms, finds maya's, and joins it.
  dawson.start(["race"]);
  await dawson.waitFor(/maya\s+[a-z]+-[a-z]+/);
  await sleep(600);
  dawson.key("\x1b[B"); // down, onto maya's room
  await shot("race-browse");
  dawson.key("\r");
  await dawson.waitFor(/3 racers/);
  await sleep(400);
  await shot("race-lobby");

  // Round one. maya starts it; the countdown hides the text until zero.
  const stops = [pump(maya), pump(sam)];
  maya.key("\r");
  await dawson.waitFor(/get ready/);
  await sleep(700);
  await shot("race-countdown");
  await dawson.waitFor(/give up/);

  const typing = [
    maya.race({ pace: 62, until: /finished|pace/ }),
    sam.race({ pace: 128, jitter: 50, until: /finished|pace/, typo: true }),
    // Over the line, dawson waits on sam.
    dawson.race({ pace: 80, until: /finished|pace/, typo: true }).then(async () => {
      await dawson.waitFor(/waiting for sam/);
      await sleep(300);
      await shot("race-finished");
    }),
  ];
  // Mid-race: maya's ghost ahead in dawson's text, sam's behind.
  await sleep(5200);
  await shot("race-typing");
  await Promise.all(typing);
  await dawson.waitFor(PODIUM, 60_000);
  await sleep(500);
  await shot("race-podium");

  // The replay, part way through.
  dawson.key("r");
  await sleep(3600);
  await shot("race-replay");
  dawson.key("\x1b");
  await dawson.waitFor(PODIUM);

  // Round two, so the podium carries a tally of wins.
  maya.key("\r");
  await dawson.waitFor(/give up/, 10_000);
  await Promise.all([
    maya.race({ pace: 70, until: /finished|pace/ }),
    sam.race({ pace: 120, jitter: 40, until: /finished|pace/ }),
    dawson.race({ pace: 64, until: /finished|pace/ }),
  ]);
  await dawson.waitFor(PODIUM, 60_000);
  await sleep(500);
  await shot("race-podium-2");
  stops.forEach((s) => s());
} finally {
  for (const r of Object.values(racers)) r.stop();
  await browser.close();
  fs.rmSync(TMP, { recursive: true, force: true });
}

log(`done: ${shots.join(", ")} in ${path.relative(process.cwd(), OUT) || "."}`);
