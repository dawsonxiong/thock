// Captures the screenshots used on the portfolio site from real thock runs.
//
// thock runs in a pseudo-terminal and its output is drawn by xterm.js in headless Chrome, framed as a
// macOS window. A scripted typist reads the words off the screen and types them at a human pace, so
// every number in the shots comes from a real test. History and config live in a throwaway directory.
//
//   pnpm capture                  # writes out/<name>.png (2x) and out/<name>.webp (1600 wide)
//   pnpm capture --out <dir>      # somewhere else
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

const COLS = 96;
const ROWS = 24;
const THEME = "tokyonight";
const WEBP_WIDTH = 1600;
// Tokyo Night's window colours, so the frame matches the theme.
const WINDOW = { background: "#1a1b26", foreground: "#c0caf5", cursor: "#7aa2f7" };
// Chromium on Linux measures JetBrains Mono's cell taller than Chrome on macOS; LINE_HEIGHT=1.125
// there gives the same rows as the published shots.
const LINE_HEIGHT = Number(process.env.LINE_HEIGHT ?? 1.25);

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const log = (...a) => console.log(new Date().toISOString().slice(11, 19), ...a);

// Build the working tree's thock and give it an empty home.
const TMP = fs.mkdtempSync(path.join(os.tmpdir(), "thock-shots-"));
const BIN = path.join(TMP, "thock");
log("building thock");
execFileSync("go", ["build", "-o", BIN, "."], { cwd: REPO, stdio: "inherit" });
fs.mkdirSync(OUT, { recursive: true });

// node-pty's prebuilt spawn-helper arrives without its executable bit when install scripts are
// skipped, and every spawn then fails with "posix_spawnp failed". Only macOS uses one.
const ptyDir = path.dirname(createRequire(import.meta.url).resolve("node-pty/package.json"));
const helper = path.join(ptyDir, "prebuilds", `${process.platform}-${process.arch}`, "spawn-helper");
if (fs.existsSync(helper)) fs.chmodSync(helper, 0o755);

// The terminal page.
const mod = (p) => fs.readFileSync(path.join(HERE, "node_modules", p), "utf8");
// CHROME points at a Chromium build when Google Chrome is not installed.
const browser = await chromium.launch(process.env.CHROME ? { executablePath: process.env.CHROME, headless: true } : { channel: "chrome", headless: true });
const page = await (await browser.newContext({ viewport: { width: 1180, height: 560 }, deviceScaleFactor: 2 })).newPage();
await page.setContent(`<!doctype html><html><head><style>${mod("@xterm/xterm/css/xterm.css")}
  html,body{margin:0;background:#0b0c14;height:100%}
  .win{position:absolute;left:24px;top:24px;border-radius:12px;background:${WINDOW.background};padding:34px 26px 22px;box-shadow:0 20px 60px #0008}
  .dots{position:absolute;left:16px;top:12px;display:flex;gap:8px}.dots i{width:12px;height:12px;border-radius:50%;display:block}
  .xterm .xterm-viewport{background:${WINDOW.background} !important}
</style></head><body><div class="win"><div class="dots"><i style="background:#ff5f57"></i><i style="background:#febc2e"></i><i style="background:#28c840"></i></div><div id="t"></div></div>
<script>${mod("@xterm/xterm/lib/xterm.js")}</script>
<script>${mod("@xterm/addon-canvas/lib/addon-canvas.js")}</script>
<script>
  const term = new Terminal({ cols: ${COLS}, rows: ${ROWS}, fontFamily: '"JetBrains Mono", Menlo, monospace', fontSize: 15, lineHeight: ${LINE_HEIGHT}, customGlyphs: true, cursorBlink: false, cursorStyle: "bar", theme: ${JSON.stringify(WINDOW)} });
  term.open(document.getElementById("t"));
  // The DOM renderer draws block elements from the font, which leaves a gap per row at this line
  // height; the canvas renderer draws them as custom glyphs that fill the whole cell.
  term.loadAddon(new CanvasAddon.CanvasAddon());
  // Unfocused, xterm draws the cursor as a hollow outline.
  term.focus();
  window.term = term;
  window.lineAt = (y) => term.buffer.active.getLine(y)?.translateToString(true) ?? "";
  window.cursor = () => ({ x: term.buffer.active.cursorX, y: term.buffer.active.cursorY });
</script></body></html>`);
await page.waitForFunction(() => !!window.term);
// The canvas sizes itself before it sees the 2x pixel ratio; nudging the font once it has painted
// forces a re-measure.
await sleep(300);
await page.evaluate(() => {
  window.term.options.fontSize = 16;
  window.term.options.fontSize = 15;
});
await page.waitForFunction(() => {
  const c = document.querySelector(".xterm-text-layer");
  return c.width === 2 * parseInt(c.style.width);
});

// The pty.
let proc = null;
let pending = "";

// thock saves its flags as the next run's defaults, so every launch spells out the full setup.
function start(flags) {
  pending = "";
  proc = pty.spawn(BIN, [...flags, "--theme", THEME], {
    name: "xterm-256color",
    cols: COLS,
    rows: ROWS,
    cwd: TMP,
    env: {
      ...process.env,
      TERM: "xterm-256color",
      COLORTERM: "truecolor",
      LANG: "en_US.UTF-8",
      XDG_DATA_HOME: path.join(TMP, "data"),
      XDG_CONFIG_HOME: path.join(TMP, "config"),
    },
  });
  proc.onData((d) => {
    pending += d;
  });
}
async function stop() {
  proc?.kill();
  proc = null;
  await sleep(150);
  pending = "";
  await page.evaluate(() => window.term.reset());
}
const key = (s) => proc.write(s);
async function flush() {
  if (!pending) return;
  const d = pending;
  pending = "";
  await page.evaluate((s) => window.term.write(s), d);
}
async function settle(ms = 250) {
  await sleep(ms);
  await flush();
  await sleep(60);
  await flush();
}
const screen = async () =>
  (await page.evaluate((rows) => Array.from({ length: rows }, (_, y) => window.lineAt(y)), ROWS)).join("\n");
async function waitFor(re, timeout = 40_000) {
  const end = Date.now() + timeout;
  while (Date.now() < end) {
    await flush();
    if (re.test(await screen())) return true;
    await sleep(250);
  }
  throw new Error(`timed out waiting for ${re}`);
}

const shots = [];
async function shot(name) {
  await settle(300);
  const png = path.join(OUT, `${name}.png`);
  await page.locator(".win").screenshot({ path: png });
  await sharp(png).resize({ width: WEBP_WIDTH }).webp({ quality: 90 }).toFile(path.join(OUT, `${name}.webp`));
  shots.push(name);
  log("shot", name);
}

// Types whatever sits at the cursor, a word at a time, re-reading the screen after each word. Stops
// after `seconds`, or as soon as `until` matches the screen. With `typo`, one early word gets a wrong
// letter so the results chart has a mistake to mark.
async function typeFromCursor({ seconds = 300, until = null, typo = false } = {}) {
  const end = Date.now() + seconds * 1000;
  let words = 0;
  let typoDone = !typo;
  while (Date.now() < end) {
    await flush();
    if (until && until.test(await screen())) return;
    const { x, y } = await page.evaluate(() => window.cursor());
    let word = (await page.evaluate((y) => window.lineAt(y), y)).slice(x).trim().split(/\s+/)[0];
    if (!word) {
      // End of the line: the next word starts the line below.
      word = (await page.evaluate((y) => window.lineAt(y + 1), y)).trim().split(/\s+/)[0];
      if (!word) {
        await sleep(200);
        continue;
      }
    }
    const text = !typoDone && words >= 2 && word.length > 3 ? ((typoDone = true), word.slice(0, 2) + "x" + word.slice(3)) : word;
    for (const ch of text + " ") {
      proc.write(ch);
      await sleep(85 + Math.random() * 30);
      if (Math.random() < 0.15) await flush();
    }
    words++;
  }
}

const RESULTS = /\bacc\b/;
const TIME_30 = ["--mode", "time", "--time", "30", "--list", "1k"];

try {
  // Start screen and mid-test; esc abandons the run so it stays out of history.
  start(TIME_30);
  await settle(1200);
  await shot("start");
  await typeFromCursor({ seconds: 7, typo: true });
  await shot("typing");
  key("\x1b");
  await settle(400);
  await stop();

  // Four 30s runs; the first, uninterrupted, is the results shot.
  start(TIME_30);
  await settle(1200);
  for (let run = 1; run <= 4; run++) {
    await typeFromCursor({ until: RESULTS, typo: run !== 3 });
    await waitFor(RESULTS);
    if (run === 1) await shot("results");
    log("30s run", run);
    key("\r");
    await settle(600);
  }
  await stop();

  // Four 25-word runs and two medium quotes, to fill out the stats screen.
  for (let run = 1; run <= 4; run++) {
    start(["--mode", "words", "--words", "25", "--list", "1k"]);
    await settle(900);
    await typeFromCursor({ until: RESULTS, typo: run === 2 });
    await waitFor(RESULTS);
    log("25-word run", run);
    await stop();
  }
  for (let run = 1; run <= 2; run++) {
    start(["--mode", "quotes", "--length", "medium"]);
    await settle(900);
    await typeFromCursor({ until: RESULTS });
    await waitFor(RESULTS);
    log("quote run", run);
    await stop();
  }

  // Stats over all ten runs.
  start(TIME_30);
  await settle(1000);
  key("\x13"); // ctrl+s
  await settle(500);
  await shot("stats");
  await stop();
} finally {
  await stop().catch(() => {});
  await browser.close();
  fs.rmSync(TMP, { recursive: true, force: true });
}

log(`done: ${shots.join(", ")} in ${path.relative(process.cwd(), OUT) || "."}`);
