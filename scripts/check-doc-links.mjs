#!/usr/bin/env node
// Offline check of the Markdown links in the repository: a repo-relative link must point at a
// file or folder that exists, and a #anchor at a heading of the (Markdown) file it names.
// External links (http, mailto, ...) are not followed.
// Usage: node scripts/check-doc-links.mjs   (run from the repository root)
import { execFileSync } from 'node:child_process';
import { existsSync, readFileSync } from 'node:fs';
import { posix } from 'node:path';
import { pathToFileURL } from 'node:url';

const SKIP_FILES = [/^\.claude\/run\//, /^\.claude\/plans\//];

export function slug(heading) {
  return heading
    .replace(/\[([^\]]*)\]\([^)]*\)/g, '$1')
    .replace(/[`*]/g, '')
    .trim()
    .toLowerCase()
    .replace(/[^\p{L}\p{N}\s_-]/gu, '')
    .replace(/\s/g, '-');
}

function withoutCode(md) {
  const kept = [];
  let fence = null;
  for (const line of md.split('\n')) {
    const f = /^\s*(```|~~~)/.exec(line);
    if (f) {
      fence = fence === null ? f[1] : fence === f[1] ? null : fence;
      kept.push('');
      continue;
    }
    kept.push(fence === null ? line.replace(/`[^`]*`/g, '') : '');
  }
  return kept.join('\n');
}

export function anchorsOf(md) {
  const seen = new Map();
  const anchors = new Set();
  const text = withoutCode(md);
  for (const m of text.matchAll(/^ {0,3}#{1,6}\s+(.+?)\s*#*\s*$/gm)) {
    const base = slug(m[1]);
    const n = seen.get(base) ?? 0;
    seen.set(base, n + 1);
    anchors.add(n === 0 ? base : `${base}-${n}`);
  }
  for (const m of md.matchAll(/\b(?:id|name)="([^"]+)"/g)) anchors.add(m[1].toLowerCase());
  return anchors;
}

const INLINE = /\[[^\]]*\]\((<[^>]*>|[^)\s]+)(?:\s+"[^"]*")?\)/g;
// A link whose text is an image, a badge: [![alt](image.png)](target.md)
const BADGE = /\[!\[[^\]]*\]\([^)]*\)\]\((<[^>]*>|[^)\s]+)(?:\s+"[^"]*")?\)/g;

export function linksOf(md) {
  const text = withoutCode(md);
  const targets = new Set();
  for (const re of [INLINE, BADGE]) for (const m of text.matchAll(re)) targets.add(m[1].replace(/^<|>$/g, ''));
  for (const m of text.matchAll(/^\s*\[(?!\^)[^\]]+\]:\s*(\S+)/gm)) targets.add(m[1]);
  return [...targets].map((target) => ({ target }));
}

function decode(s, decoder) {
  try {
    return decoder(s);
  } catch {
    return null;
  }
}

export function checkFile(file, content, { read, exists }) {
  const problems = [];
  const broken = (target, reason) => problems.push({ file, target, reason });
  for (const { target } of linksOf(content)) {
    if (/^[a-z][a-z0-9+.-]*:/i.test(target) || target.includes('{') || target.startsWith('//')) continue;
    const [rawPath = '', rawAnchor] = target.split('#');
    const path = decode(rawPath.split('?')[0], decodeURI);
    const anchor = rawAnchor === undefined ? undefined : decode(rawAnchor, decodeURIComponent);
    if (path === null || anchor === null) {
      broken(target, 'malformed % escape');
      continue;
    }
    const resolved = path === '' ? file : path.startsWith('/') ? path.slice(1) : posix.join(posix.dirname(file), path);
    let clean = resolved.replace(/\/$/, '') || '.';
    // The published site serves page.md as page.html; both spellings are fine.
    if (!exists(clean) && clean.endsWith('.html') && exists(clean.replace(/\.html$/, '.md'))) {
      clean = clean.replace(/\.html$/, '.md');
    }
    if (!exists(clean)) {
      broken(target, 'no such file or folder');
      continue;
    }
    if (anchor !== undefined && anchor !== '' && clean.endsWith('.md')) {
      const body = clean === file ? content : read(clean);
      if (body !== undefined && !anchorsOf(body).has(anchor.toLowerCase())) broken(target, 'no such heading');
    }
  }
  return problems;
}

export function isMain(metaUrl, argv1) {
  return argv1 !== undefined && metaUrl === pathToFileURL(argv1).href;
}

if (isMain(import.meta.url, process.argv[1])) {
  const files = execFileSync('git', ['ls-files', '*.md'], { encoding: 'utf8' })
    .split('\n')
    .filter((f) => f && !SKIP_FILES.some((re) => re.test(f)));
  const read = (p) => (existsSync(p) ? readFileSync(p, 'utf8') : undefined);
  const problems = files.flatMap((f) => checkFile(f, readFileSync(f, 'utf8'), { read, exists: existsSync }));
  for (const p of problems) console.error(`${p.file}: ${p.target}: ${p.reason}`);
  console.log(`${files.length} Markdown files checked, ${problems.length} broken links.`);
  if (problems.length > 0) process.exit(1);
}
