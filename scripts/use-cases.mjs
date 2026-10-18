#!/usr/bin/env node
// Checks the use-case catalogue (docs/use-cases.yml) against the tests. Fails on a use-case ID
// that is not in the catalogue and on an E2E test without an ID; prints the use cases no test
// covers. E2E tests carry the ID in the title (`@001-US1`), Go tests in a comment line
// (`// @001-US1`) directly above the test function.
// Usage: node scripts/use-cases.mjs   (run from the repository root)
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';

const ID = /^\d{3}-US\d+$/;
const FIELDS = ['title', 'actor', 'priority', 'status'];
const TAG = /@(\d{3}-US\d+)/g;

export function parseCatalogue(text) {
  const cases = [];
  let current = null;
  for (const raw of text.split('\n')) {
    const line = raw.replace(/\s+$/, '');
    if (line === '' || line.trimStart().startsWith('#')) continue;
    const start = /^- id:\s*(.+)$/.exec(line);
    if (start) {
      current = { id: unquote(start[1]) };
      cases.push(current);
      continue;
    }
    const field = /^ {2}(\w+):\s*(.+)$/.exec(line);
    if (!field || !current) throw new Error(`cannot read line: ${raw}`);
    current[field[1]] = unquote(field[2]);
  }
  const seen = new Set();
  for (const c of cases) {
    if (!ID.test(c.id)) throw new Error(`bad ID ${JSON.stringify(c.id)}: use NNN-USn, for example 001-US1`);
    if (seen.has(c.id)) throw new Error(`duplicate ID ${c.id}`);
    seen.add(c.id);
    for (const f of FIELDS) if (!c[f]) throw new Error(`${c.id} has no ${f}`);
  }
  return cases;
}

function unquote(s) {
  const t = s.trim();
  return /^(["']).*\1$/.test(t) ? t.slice(1, -1) : t;
}

const tagsOf = (title) => [...title.matchAll(TAG)].map((m) => m[1]);

const TITLE_START = /(?<![.\w])test(\.(?:skip|only|fixme|fail|slow))?\(\s*/g;
const LITERAL = /(['"`])((?:\\.|(?!\1).)*)\1/y;

// Tests found in an E2E spec: the first argument of test(...), also test.only/skip/fixme, with a
// title joined by `+` read as one. A skipped test is listed but does not cover a use case.
export function e2eTests(source) {
  const tests = [];
  for (const start of source.matchAll(TITLE_START)) {
    let at = start.index + start[0].length;
    let title = '';
    for (;;) {
      LITERAL.lastIndex = at;
      const literal = LITERAL.exec(source);
      if (!literal) break;
      title += literal[2];
      at = LITERAL.lastIndex;
      const plus = /\s*\+\s*/y;
      plus.lastIndex = at;
      if (!plus.exec(source)) break;
      at = plus.lastIndex;
    }
    if (title === '') continue;
    const skipped = ['.skip', '.fixme'].includes(start[1] ?? '');
    tests.push({ title, tags: tagsOf(title), skipped });
  }
  return tests;
}

// Tags in a Go test file: every `// @...` comment line. A tag must be a valid ID and sit
// directly above a Test function (stacked tags are fine).
export function goTags(source) {
  const tags = [];
  const problems = [];
  const lines = source.split('\n');
  lines.forEach((line, i) => {
    const m = /^\s*\/\/\s*@(\S+)/.exec(line);
    if (!m) return;
    const tag = m[1];
    if (!ID.test(tag)) {
      problems.push(`line ${i + 1}: bad use-case ID @${tag}: use NNN-USn, for example 001-US1`);
      return;
    }
    let next = i + 1;
    while (/^\s*\/\/\s*@\S+/.test(lines[next] ?? '')) next++;
    if (/^func Test\w*\(/.test(lines[next] ?? '')) tags.push(tag);
    else problems.push(`line ${i + 1}: @${tag} must be directly above a Test function`);
  });
  return { tags, problems };
}

export function check(cases, e2e, go) {
  const known = new Set(cases.map((c) => c.id));
  const problems = [];
  const covered = new Set();
  for (const t of e2e) {
    if (t.tags.length === 0) problems.push(`${t.file}: E2E test without a use-case ID: ${t.title}`);
    for (const id of t.tags) {
      if (known.has(id)) {
        if (!t.skipped) covered.add(id);
      } else problems.push(`${t.file}: unknown use-case ID ${id} in: ${t.title}`);
    }
  }
  for (const g of go) {
    if (known.has(g.tag)) covered.add(g.tag);
    else problems.push(`${g.file}: unknown use-case ID ${g.tag}`);
  }
  return { problems, uncovered: cases.map((c) => c.id).filter((id) => !covered.has(id)) };
}

function walk(dir, match, out = []) {
  for (const name of readdirSync(dir)) {
    if (name === 'node_modules' || name.startsWith('.')) continue;
    const path = join(dir, name);
    if (statSync(path).isDirectory()) walk(path, match, out);
    else if (match(name)) out.push(path);
  }
  return out;
}

export function isMain(metaUrl, argv1) {
  return argv1 !== undefined && metaUrl === pathToFileURL(argv1).href;
}

if (isMain(import.meta.url, process.argv[1])) {
  const cases = parseCatalogue(readFileSync('docs/use-cases.yml', 'utf8'));
  const e2e = walk('frontend/e2e', (n) => n.endsWith('.spec.ts')).flatMap((file) =>
    e2eTests(readFileSync(file, 'utf8')).map((t) => ({ file, ...t })),
  );
  const go = [];
  const goProblems = [];
  for (const file of walk('backend', (n) => n.endsWith('_test.go'))) {
    const { tags, problems } = goTags(readFileSync(file, 'utf8'));
    go.push(...tags.map((tag) => ({ file, tag })));
    goProblems.push(...problems.map((p) => `${file}: ${p}`));
  }
  const checked = check(cases, e2e, go);
  const problems = [...goProblems, ...checked.problems];
  const uncovered = checked.uncovered;
  for (const p of problems) console.error(p);
  const titles = new Map(cases.map((c) => [c.id, c.title]));
  console.log(`${cases.length - uncovered.length} of ${cases.length} use cases have a test.`);
  if (uncovered.length > 0) {
    console.log('Without a test:');
    for (const id of uncovered) console.log(`  ${id} ${titles.get(id)}`);
  }
  if (problems.length > 0) process.exit(1);
}
