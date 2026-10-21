import { test } from 'node:test';
import assert from 'node:assert/strict';
import { changelogSection } from './changelog-section.mjs';

const md = `# Changelog

## [Unreleased]

### Added

- **Next**: later.

## [1.7.0-omk.2] - 2026-10-17

### Fixed

- **Two**: fixed.

## [1.7.0-omk.1] - 2026-10-16

### Added

- **One**: added.

[1.7.0-omk.2]: https://example.com/compare
`;

test('returns the body of the version section without its heading', () => {
  assert.equal(changelogSection(md, 'v1.7.0-omk.2'), '### Fixed\n\n- **Two**: fixed.');
});

test('the last section stops before the link references', () => {
  assert.equal(changelogSection(md, '1.7.0-omk.1'), '### Added\n\n- **One**: added.');
});

test('a missing or empty section throws', () => {
  assert.throws(() => changelogSection(md, 'v9.9.9-omk.1'), /no section/);
  assert.throws(() => changelogSection('## [1.0.0-omk.1] - x\n\n## [0.9.0] - y\n', '1.0.0-omk.1'), /no section/);
});
