import { test } from 'node:test';
import assert from 'node:assert/strict';
import { checkSubject } from './check-commits.mjs';

test('accepts conventional subjects', () => {
  for (const s of [
    'feat: add trash',
    'fix(api): stop a 500',
    'refactor!: drop the old route',
    'chore: bump x',
    'docs: explain y',
  ]) {
    assert.equal(checkSubject(s), null, s);
  }
});

test('accepts merge and revert commits, but not any subject that starts with Merge', () => {
  assert.equal(checkSubject('Merge pull request #12 from a/b'), null);
  assert.equal(checkSubject("Merge branch 'main' into omk"), null);
  assert.equal(checkSubject("Merge remote-tracking branch 'origin/main'"), null);
  assert.equal(checkSubject('Revert "feat: add trash"'), null);
  assert.match(checkSubject('Merge the two lists'), /conventional/);
});

test('the length of a Dependabot subject is not checked, its prefix is', () => {
  const long = 'build(deps): bump github.com/some/very-long-module-name from 1.2.3 to 1.2.4 in /backend';
  assert.ok(long.length > 72);
  assert.match(checkSubject(long), /72/);
  assert.equal(checkSubject(long, { author: 'dependabot[bot]' }), null);
  assert.match(checkSubject('Bump x from 1 to 2', { author: 'dependabot[bot]' }), /conventional/);
});

test('rejects a missing or unknown prefix', () => {
  assert.match(checkSubject('Add trash'), /conventional/);
  assert.match(checkSubject('feature: add trash'), /conventional/);
  assert.match(checkSubject('feat add trash'), /conventional/);
  assert.match(checkSubject('feat:add trash'), /conventional/);
  assert.match(checkSubject('feat: '), /conventional/);
});

test('rejects a subject longer than 72 characters', () => {
  const long = 'feat: ' + 'x'.repeat(67);
  assert.equal(long.length, 73);
  assert.match(checkSubject(long), /72/);
  assert.equal(checkSubject('feat: ' + 'x'.repeat(66)), null);
});
