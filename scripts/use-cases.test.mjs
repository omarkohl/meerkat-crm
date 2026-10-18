import { test } from 'node:test';
import assert from 'node:assert/strict';
import { parseCatalogue, e2eTests, goTags, check } from './use-cases.mjs';

const yml = `# comment
- id: 001-US1
  title: Register an account
  actor: visitor
  priority: high
  status: implemented

- id: 001-US2
  title: "Log in"
  actor: user
  priority: high
  status: implemented
`;

test('parseCatalogue reads the list of use cases', () => {
  const cases = parseCatalogue(yml);
  assert.deepEqual(
    cases.map((c) => [c.id, c.title, c.actor, c.priority, c.status]),
    [
      ['001-US1', 'Register an account', 'visitor', 'high', 'implemented'],
      ['001-US2', 'Log in', 'user', 'high', 'implemented'],
    ],
  );
});

test('parseCatalogue rejects a bad ID, a missing field and a duplicate', () => {
  assert.throws(
    () => parseCatalogue('- id: 1-US1\n  title: x\n  actor: a\n  priority: high\n  status: implemented\n'),
    /ID/,
  );
  assert.throws(() => parseCatalogue('- id: 001-US1\n  title: x\n'), /actor/);
  assert.throws(() => parseCatalogue(yml + yml), /duplicate/);
});

test('e2eTests lists titles with their tags and finds the untagged ones', () => {
  const src = `test.describe('X', () => {
  test('does a thing @001-US1', async () => {});
  test("both @001-US1 @001-US2", async () => {});
  test('has no tag', async () => {});
});`;
  assert.deepEqual(
    e2eTests(src).map((t) => [t.title, t.tags, t.skipped]),
    [
      ['does a thing @001-US1', ['001-US1'], false],
      ['both @001-US1 @001-US2', ['001-US1', '001-US2'], false],
      ['has no tag', [], false],
    ],
  );
});

test('e2eTests ignores other .test( calls, reads skipped tests and joined titles', () => {
  const src = `const ok = /x/.test('abc');
  await test.skip('skipped @001-US1', async () => {});
  test.fixme("fixme @001-US2", async () => {});
  test('joined ' + 'title @001-US3', async () => {});
  test.only('only @001-US4', async () => {});`;
  assert.deepEqual(
    e2eTests(src).map((t) => [t.title, t.tags, t.skipped]),
    [
      ['skipped @001-US1', ['001-US1'], true],
      ['fixme @001-US2', ['001-US2'], true],
      ['joined title @001-US3', ['001-US3'], false],
      ['only @001-US4', ['001-US4'], false],
    ],
  );
});

test('goTags reads the tag comments above a test, also stacked', () => {
  const src = `package x\n\n// @001-US2\n// @001-US3\nfunc TestLogin(t *testing.T) {}\n\nfunc TestOther(t *testing.T) {}\n`;
  assert.deepEqual(goTags(src), { tags: ['001-US2', '001-US3'], problems: [] });
});

test('goTags reports a bad ID and a tag that is not directly above a test', () => {
  const bad = goTags(`// @01-US1\nfunc TestA(t *testing.T) {}\n`);
  assert.equal(bad.problems.length, 1);
  assert.match(bad.problems[0], /01-US1/);
  const apart = goTags(`// @001-US1\n\nfunc TestA(t *testing.T) {}\n`);
  assert.equal(apart.problems.length, 1);
  assert.match(apart.problems[0], /directly above/);
  const helper = goTags(`// @001-US1\nfunc helper() {}\n`);
  assert.equal(helper.problems.length, 1);
});

test('a skipped test does not cover a use case', () => {
  const cases = parseCatalogue(yml);
  const r = check(cases, [{ file: 'a.spec.ts', title: 't', tags: ['001-US1'], skipped: true }], []);
  assert.deepEqual(r.uncovered, ['001-US1', '001-US2']);
});

test('check fails on unknown IDs and untagged E2E tests, and lists the uncovered', () => {
  const cases = parseCatalogue(yml);
  const ok = check(
    cases,
    [{ file: 'a.spec.ts', title: 't', tags: ['001-US1'] }],
    [{ file: 'a_test.go', tag: '001-US1' }],
  );
  assert.deepEqual(ok.problems, []);
  assert.deepEqual(ok.uncovered, ['001-US2']);

  const bad = check(
    cases,
    [
      { file: 'a.spec.ts', title: 'no tag', tags: [] },
      { file: 'a.spec.ts', title: 'typo', tags: ['001-US9'] },
    ],
    [{ file: 'a_test.go', tag: '009-US1' }],
  );
  assert.equal(bad.problems.length, 3);
  assert.match(bad.problems.join('\n'), /no tag/);
  assert.match(bad.problems.join('\n'), /001-US9/);
  assert.match(bad.problems.join('\n'), /009-US1/);
});
