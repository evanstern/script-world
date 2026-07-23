#!/usr/bin/env node
// check-freshness.mjs — read-only freshness reporter for docs/player/.
//
// Contract: specs/026-player-docs/contracts/provenance-and-check.md
//
// Node >= 18, ESM, zero npm dependencies (stdlib fs/path/child_process only).
// This script NEVER writes any file. It only reads and reports.
//
// Usage:
//   node check-freshness.mjs [--check] [--json]
//
// Exit codes:
//   0  every expected page exists, parses, and every source pin matches
//   1  at least one page is stale, missing, or broken-ref
//   2  usage/environment error (not a git repo, docs/player unreadable, bad flag)

import { existsSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { execFileSync } from 'node:child_process';

// ---------------------------------------------------------------------------
// Argument parsing
// ---------------------------------------------------------------------------

const KNOWN_FLAGS = new Set(['--check', '--json']);

function parseArgs(argv) {
  const flags = { check: false, json: false };
  for (const arg of argv) {
    if (!KNOWN_FLAGS.has(arg)) {
      process.stderr.write(`check-freshness: unknown flag ${JSON.stringify(arg)}\n`);
      process.exit(2);
    }
    if (arg === '--check') flags.check = true;
    if (arg === '--json') flags.json = true;
  }
  return flags;
}

// ---------------------------------------------------------------------------
// Repo root + expected page set (data-model.md: expected page set)
// ---------------------------------------------------------------------------

function resolveRepoRoot() {
  try {
    const out = execFileSync('git', ['rev-parse', '--show-toplevel'], {
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'pipe'],
    });
    return out.trim();
  } catch {
    process.stderr.write('check-freshness: not inside a git repository\n');
    process.exit(2);
  }
}

// The seven slugs from plan.md's Project Structure. Adding a page means
// adding its slug here — a one-line, reviewed change (data-model.md).
const EXPECTED_PAGES = [
  'index.html',
  'getting-started.html',
  'playing-via-metatron.html',
  'time-and-speed.html',
  'reading-the-story.html',
  'the-ai-behind-the-village.html',
  'llm-setup-basics.html',
];

const GENERATED_BY_RE =
  /^\s*<meta\s+name="promptworld-docs:generated-by"\s+content="([^"]*)">\s*$/;
const SOURCE_RE = /^\s*<meta\s+name="promptworld-docs:source"\s+content="([^"]*)">\s*$/;
const SOURCE_CONTENT_RE = /^(.+)@([0-9a-f]{40})$/;

// ---------------------------------------------------------------------------
// Current-pin resolution (research.md D2 / data-model.md "Source (two kinds)")
// ---------------------------------------------------------------------------

function currentPinForWikiNote(repoRoot, relPath) {
  const abs = join(repoRoot, relPath);
  if (!existsSync(abs)) return { ok: false, reason: `${relPath}: file not found` };
  const text = readFileSync(abs, 'utf8');
  const fmMatch = text.match(/^---\r?\n([\s\S]*?)\r?\n---/);
  if (!fmMatch) return { ok: false, reason: `${relPath}: no frontmatter` };
  const vaMatch = fmMatch[1].match(/^verified_against:\s*(\S+)\s*$/m);
  if (!vaMatch) return { ok: false, reason: `${relPath}: no verified_against` };
  return { ok: true, pin: vaMatch[1].toLowerCase() };
}

function currentPinForPlainFile(repoRoot, relPath) {
  const abs = join(repoRoot, relPath);
  if (!existsSync(abs)) return { ok: false, reason: `${relPath}: file not found` };
  let out;
  try {
    out = execFileSync('git', ['log', '-1', '--format=%H', '--', relPath], {
      cwd: repoRoot,
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'pipe'],
    }).trim();
  } catch {
    return { ok: false, reason: `${relPath}: git log failed` };
  }
  if (!out) return { ok: false, reason: `${relPath}: no git history` };
  return { ok: true, pin: out.toLowerCase() };
}

function currentPinFor(repoRoot, relPath) {
  return relPath.startsWith('docs/wiki/')
    ? currentPinForWikiNote(repoRoot, relPath)
    : currentPinForPlainFile(repoRoot, relPath);
}

function shortPin(pin) {
  return pin && pin.length >= 7 ? `${pin.slice(0, 7)}…` : pin;
}

// ---------------------------------------------------------------------------
// Per-page evaluation
// ---------------------------------------------------------------------------

function evaluatePage(repoRoot, playerDirExists, page) {
  const abs = join(repoRoot, 'docs/player', page);

  if (!playerDirExists || !existsSync(abs)) {
    return { page, verdict: 'missing', reasons: [], sources: [] };
  }

  let text;
  try {
    text = readFileSync(abs, 'utf8');
  } catch (err) {
    if (err && err.code === 'EACCES') {
      process.stderr.write(`check-freshness: cannot read ${abs}: ${err.message}\n`);
      process.exit(2);
    }
    return { page, verdict: 'missing', reasons: [], sources: [] };
  }

  const lines = text.split(/\r?\n/);
  let generatedBy = null;
  const sourceTags = [];
  for (const line of lines) {
    const gb = line.match(GENERATED_BY_RE);
    if (gb) generatedBy = gb[1];
    const src = line.match(SOURCE_RE);
    if (src) sourceTags.push(src[1]);
  }

  const brokenReasons = [];
  if (!generatedBy) {
    brokenReasons.push('missing promptworld-docs:generated-by meta');
  }

  const isIndex = page === 'index.html';
  if (isIndex) {
    if (sourceTags.length > 0) {
      brokenReasons.push('index.html must not declare promptworld-docs:source tags');
    }
    if (brokenReasons.length > 0) {
      return { page, verdict: 'broken-ref', reasons: brokenReasons, sources: [] };
    }
    return { page, verdict: 'fresh', reasons: [], sources: [] };
  }

  if (sourceTags.length === 0) {
    brokenReasons.push('no promptworld-docs:source tags declared');
  }

  const sources = [];
  const staleReasons = [];
  for (const content of sourceTags) {
    const m = content.match(SOURCE_CONTENT_RE);
    if (!m) {
      brokenReasons.push(`malformed source tag: ${content}`);
      continue;
    }
    const [, relPath, recordedPin] = m;
    const current = currentPinFor(repoRoot, relPath);
    if (!current.ok) {
      brokenReasons.push(current.reason);
      sources.push({ path: relPath, recorded: recordedPin, current: null, fresh: false });
      continue;
    }
    const fresh = recordedPin.toLowerCase() === current.pin;
    sources.push({ path: relPath, recorded: recordedPin, current: current.pin, fresh });
    if (!fresh) {
      staleReasons.push(
        `${relPath} moved ${shortPin(recordedPin)} → ${shortPin(current.pin)}`
      );
    }
  }

  if (brokenReasons.length > 0) {
    return { page, verdict: 'broken-ref', reasons: brokenReasons, sources };
  }
  if (staleReasons.length > 0) {
    return { page, verdict: 'stale', reasons: staleReasons, sources };
  }
  return { page, verdict: 'fresh', reasons: [], sources };
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

function main() {
  const flags = parseArgs(process.argv.slice(2));
  const repoRoot = resolveRepoRoot();

  const playerDir = join(repoRoot, 'docs/player');
  let playerDirExists = existsSync(playerDir);
  if (playerDirExists) {
    try {
      if (!statSync(playerDir).isDirectory()) playerDirExists = false;
    } catch (err) {
      if (err && err.code === 'EACCES') {
        process.stderr.write(`check-freshness: cannot read ${playerDir}: ${err.message}\n`);
        process.exit(2);
      }
      playerDirExists = false;
    }
  }

  const results = EXPECTED_PAGES.map((page) => evaluatePage(repoRoot, playerDirExists, page));

  const summary = { fresh: 0, stale: 0, missing: 0, brokenRef: 0 };
  for (const r of results) {
    if (r.verdict === 'fresh') summary.fresh += 1;
    else if (r.verdict === 'stale') summary.stale += 1;
    else if (r.verdict === 'missing') summary.missing += 1;
    else if (r.verdict === 'broken-ref') summary.brokenRef += 1;
  }

  if (flags.json) {
    const report = {
      pages: results.map((r) => ({
        page: r.page,
        verdict: r.verdict,
        sources: r.sources,
      })),
      summary,
    };
    process.stdout.write(`${JSON.stringify(report, null, 2)}\n`);
  } else {
    for (const r of results) {
      const detail = r.reasons.length > 0 ? `  ${r.reasons.join('; ')}` : '';
      process.stdout.write(`${r.verdict}  ${r.page}${detail}\n`);
    }
    process.stdout.write(
      `${summary.fresh} fresh, ${summary.stale} stale, ${summary.missing} missing, ` +
        `${summary.brokenRef} broken-ref\n`
    );
  }

  const allFresh = summary.stale === 0 && summary.missing === 0 && summary.brokenRef === 0;
  process.exit(allFresh ? 0 : 1);
}

main();
