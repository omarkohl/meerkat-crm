#!/usr/bin/env node
// Prints the CHANGELOG.md section of a release tag, for the GitHub release notes.
// Usage: node scripts/changelog-section.mjs v1.8.0-omk.1   (run from the repository root)
import { readFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';

export function changelogSection(md, tag) {
  const version = tag.replace(/^v/, '');
  const lines = md.split('\n');
  const start = lines.findIndex((l) => l.startsWith(`## [${version}]`));
  const body = [];
  if (start >= 0) {
    for (const line of lines.slice(start + 1)) {
      if (/^## \[|^\[[^\]]+\]: /.test(line)) break;
      body.push(line);
    }
  }
  const text = body.join('\n').trim();
  if (!text) throw new Error(`CHANGELOG.md has no section for ${version}`);
  return text;
}

if (import.meta.url === pathToFileURL(process.argv[1]).href) {
  try {
    console.log(changelogSection(readFileSync('CHANGELOG.md', 'utf8'), process.argv[2] ?? ''));
  } catch (e) {
    console.error(e.message);
    process.exit(1);
  }
}
