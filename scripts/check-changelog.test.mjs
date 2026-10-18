import { test } from 'node:test';
import assert from 'node:assert/strict';
import { checkChangelog } from './check-changelog.mjs';

const wrap = (body) =>
  `# Changelog\n\n## [Unreleased]\n\n${body}\n## [1.0.0] - 2026-01-01\n\n### Added\n\n- old bullet without bold\n`;

test('accepts a well-formed entry with and without an issue', () => {
  const md = wrap('### Fixed\n\n- **Saving works** (#12): it failed before.\n- **Another fix**: one line.\n');
  assert.deepEqual(checkChangelog(md), []);
});

test('only the Unreleased section is checked: released entries are history', () => {
  assert.deepEqual(checkChangelog(wrap('')), []);
  const released = '# C\n\n## [Unreleased]\n\n## [1.0.0-omk.1] - 2026-01-01\n\n### Added\n\n- no bold here\n';
  assert.deepEqual(checkChangelog(released), []);
});

test('rejects an entry without a bold lead phrase', () => {
  const problems = checkChangelog(wrap('### Fixed\n\n- Saving works again.\n'));
  assert.equal(problems.length, 1);
  assert.match(problems[0], /bold/);
});

test('rejects a bad issue reference and a missing colon', () => {
  assert.match(checkChangelog(wrap('### Fixed\n\n- **A** (#abc): text.\n')).join(), /issue/);
  assert.match(checkChangelog(wrap('### Fixed\n\n- **A** text without colon.\n')).join(), /colon/);
});

test('rejects more than 40 words, a sub-bullet and a second paragraph', () => {
  const long = Array.from({ length: 41 }, (_, i) => `w${i}`).join(' ');
  assert.match(checkChangelog(wrap(`### Fixed\n\n- **A**: ${long}\n`)).join(), /words/);
  assert.match(checkChangelog(wrap('### Fixed\n\n- **A**: text.\n  - sub\n')).join(), /sub-bullet/);
  assert.match(checkChangelog(wrap('### Fixed\n\n- **A**: text.\n\n  Second paragraph.\n')).join(), /paragraph/);
});

test('Internal entries are short and at most three', () => {
  const ok = wrap('### Internal\n\n- **One**: short.\n- **Two**: short.\n');
  assert.deepEqual(checkChangelog(ok), []);
  const wordy = Array.from({ length: 26 }, (_, i) => `w${i}`).join(' ');
  assert.match(checkChangelog(wrap(`### Internal\n\n- **One**: ${wordy}\n`)).join(), /Internal/);
  const many = wrap('### Internal\n\n' + [1, 2, 3, 4, 5].map((n) => `- **E${n}**: short.\n`).join(''));
  const problems = checkChangelog(many).filter((p) => /Internal/.test(p));
  assert.equal(problems.length, 1, 'reported once');
  assert.deepEqual(
    checkChangelog(wrap('### Internal\n\n' + [1, 2, 3].map((n) => `- **E${n}**: short.\n`).join(''))),
    [],
  );
});

test('rejects an unindented continuation line and a star sub-bullet', () => {
  assert.match(checkChangelog(wrap('### Fixed\n\n- **A**: text\ncontinued.\n')).join(), /indent/);
  assert.match(checkChangelog(wrap('### Fixed\n\n- **A**: text.\n  * sub\n')).join(), /sub-bullet/);
});

test('a bullet before any section heading says so', () => {
  const md = '# C\n\n## [Unreleased]\n\n- **A**: text.\n';
  const problems = checkChangelog(md);
  assert.equal(problems.length, 1);
  assert.match(problems[0], /section/);
  assert.doesNotMatch(problems[0], /null/);
});

test('rejects unknown section names', () => {
  assert.match(checkChangelog(wrap('### Stuff\n\n- **A**: text.\n')).join(), /section/);
});
