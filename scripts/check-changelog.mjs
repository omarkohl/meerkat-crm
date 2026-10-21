#!/usr/bin/env node
// Checks the [Unreleased] entries of CHANGELOG.md against the AGENTS.md changelog rules.
// Usage: node scripts/check-changelog.mjs   (run from the repository root)
import { readFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';

const SECTIONS = ['Added', 'Changed', 'Deprecated', 'Removed', 'Fixed', 'Security', 'Internal'];
export const MAX_WORDS = 40;
export const MAX_INTERNAL_WORDS = 25;
export const MAX_INTERNAL_ENTRIES = 3;

const checked = (heading) => /Unreleased/i.test(heading);

export function checkChangelog(md) {
  const problems = [];
  let version = null;
  let section = null;
  let entry = null;
  let internal = 0;
  const finish = () => {
    if (!entry) return;
    const words = entry.text.trim().split(/\s+/).length;
    const where = `${version} / ${section}: "${entry.text.slice(0, 40).trim()}..."`;
    const limit = section === 'Internal' ? MAX_INTERNAL_WORDS : MAX_WORDS;
    if (words > limit)
      problems.push(
        `${where} has ${words} words, the limit is ${limit}${section === 'Internal' ? ' for an Internal entry' : ''}`,
      );
    entry = null;
  };
  const endSection = () => {
    finish();
    if (section === 'Internal' && internal > MAX_INTERNAL_ENTRIES) {
      problems.push(
        `${version}: ${internal} Internal entries, the limit is ${MAX_INTERNAL_ENTRIES}; keep only the valuable or disruptive ones`,
      );
    }
    internal = 0;
  };
  const lines = md.split('\n');
  lines.forEach((line, i) => {
    const h2 = /^## \[([^\]]+)\]/.exec(line);
    if (h2) {
      endSection();
      version = h2[1];
      section = null;
      return;
    }
    if (version === null || !checked(version)) return;
    const h3 = /^### (.+)$/.exec(line);
    if (h3) {
      endSection();
      section = h3[1].trim();
      if (!SECTIONS.includes(section))
        problems.push(`${version}: unknown section "${section}" (use ${SECTIONS.join(', ')})`);
      return;
    }
    if (/^- /.test(line)) {
      finish();
      if (section === null) {
        problems.push(
          `${version}: line ${i + 1} is an entry before any section; put it under ### Added, Changed, Fixed, ...`,
        );
        return;
      }
      const next = lines[i + 1] ?? '';
      const head = /^ {2}\(https:/.test(next) ? `${line} ${next.trim()}` : line;
      const m = /^- \*\*([^*]+)\*\*(?: \(([^)]*)\))?(:?)/.exec(head);
      const where = `${version} / ${section}: line ${i + 1}`;
      if (!m) {
        problems.push(`${where} needs a bold lead phrase: - **Lead phrase**: text`);
        return;
      }
      if (m[2] !== undefined && !/^https:\/\/github\.com\/[^/\s]+\/[^/\s]+\/issues\/\d+$/.test(m[2]))
        problems.push(`${where} has a bad issue reference (${m[2]}): write the full issue URL`);
      if (m[3] !== ':') problems.push(`${where} needs a colon after the lead phrase`);
      if (section === 'Internal') internal++;
      entry = { text: line.slice(2) };
      return;
    }
    if (entry && /^ {2}[-*+] /.test(line)) {
      problems.push(`${version} / ${section}: line ${i + 1} is a sub-bullet; one bullet per change`);
      return;
    }
    if (entry && /^ {2}\S/.test(line)) {
      entry.text += ' ' + line.trim();
      return;
    }
    if (entry && line.trim() !== '' && !/^#/.test(line)) {
      problems.push(`${version} / ${section}: line ${i + 1} continues an entry without a two-space indent`);
      return;
    }
    if (entry && line.trim() === '' && /^ {2}\S/.test(lines[i + 1] ?? '')) {
      problems.push(`${version} / ${section}: line ${i + 2} is a second paragraph; one bullet is one paragraph`);
    }
  });
  endSection();
  return problems;
}

export function isMain(metaUrl, argv1) {
  return argv1 !== undefined && metaUrl === pathToFileURL(argv1).href;
}

if (isMain(import.meta.url, process.argv[1])) {
  const problems = checkChangelog(readFileSync('CHANGELOG.md', 'utf8'));
  for (const p of problems) console.error(p);
  console.log(`CHANGELOG.md checked, ${problems.length} problems.`);
  if (problems.length > 0) process.exit(1);
}
