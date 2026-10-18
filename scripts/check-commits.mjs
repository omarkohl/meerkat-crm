#!/usr/bin/env node
// Checks commit subjects: a conventional prefix and at most 72 characters (AGENTS.md). Merge and
// revert commits, and the length of Dependabot subjects, are not checked.
// Usage: node scripts/check-commits.mjs <git revision range>   e.g. origin/main..HEAD
import { execFileSync } from 'node:child_process';
import { pathToFileURL } from 'node:url';

const TYPES = ['feat', 'fix', 'refactor', 'test', 'docs', 'chore', 'perf', 'build', 'ci', 'style', 'revert'];
const SUBJECT = new RegExp(`^(${TYPES.join('|')})(\\([^)]+\\))?!?: \\S`);
export const MAX_LENGTH = 72;

export function checkSubject(subject, { author = '' } = {}) {
  if (/^Merge (pull request|branch|remote-tracking branch) /.test(subject) || /^Revert "/.test(subject)) return null;
  if (!SUBJECT.test(subject)) {
    return `not a conventional subject (type: description, type one of ${TYPES.join(', ')}): ${subject}`;
  }
  if (subject.length > MAX_LENGTH && author !== 'dependabot[bot]')
    return `longer than ${MAX_LENGTH} characters (${subject.length}): ${subject}`;
  return null;
}

export function isMain(metaUrl, argv1) {
  return argv1 !== undefined && metaUrl === pathToFileURL(argv1).href;
}

if (isMain(import.meta.url, process.argv[1])) {
  const range = process.argv[2];
  if (!range) {
    console.error('usage: check-commits.mjs <git revision range>');
    process.exit(2);
  }
  const rows = execFileSync('git', ['log', '--format=%an%x09%s', range], { encoding: 'utf8' })
    .split('\n')
    .filter(Boolean);
  const subjects = rows.map((r) => r.slice(r.indexOf('\t') + 1));
  const problems = rows
    .map((r) => checkSubject(r.slice(r.indexOf('\t') + 1), { author: r.slice(0, r.indexOf('\t')) }))
    .filter(Boolean);
  for (const p of problems) console.error(p);
  console.log(`${subjects.length} commit subjects checked, ${problems.length} problems.`);
  if (problems.length > 0) process.exit(1);
}
