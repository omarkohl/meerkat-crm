import { test } from 'node:test';
import assert from 'node:assert/strict';
import { slug, anchorsOf, linksOf, checkFile } from './check-doc-links.mjs';

test('slug follows the GitHub rules', () => {
  assert.equal(slug('Running E2E Tests'), 'running-e2e-tests');
  assert.equal(slug('Modules (hexagonal)'), 'modules-hexagonal');
  assert.equal(slug('`make` targets & more'), 'make-targets--more');
});

test('anchorsOf numbers repeated headings and skips code fences', () => {
  const md = '# A\n## B\n## B\n```\n# not a heading\n```\n';
  assert.deepEqual([...anchorsOf(md)].sort(), ['a', 'b', 'b-1']);
});

test('linksOf finds inline and reference links but not code', () => {
  const md =
    'See [a](x.md#y) and [b](<z w.md>) and `[c](nope.md)`.\n\n```\n[d](nope2.md)\n```\n\n[ref]: docs/r.md\n[web](https://example.com)\n';
  assert.deepEqual(
    linksOf(md).map((l) => l.target),
    ['x.md#y', 'z w.md', 'https://example.com', 'docs/r.md'],
  );
});

const files = {
  'docs/a.md':
    '# A\n[ok](b.md#top) [bad file](missing.md) [bad anchor](b.md#nope) [self](#a) [self bad](#zzz) [dir](../docs) [ext](https://x.y) [root](/docs/b.md)\n',
  'docs/b.md': '# Top\n',
};
const read = (p) => files[p];
const exists = (p) => p in files || p === 'docs';

test('checkFile reports broken files and anchors only', () => {
  const problems = checkFile('docs/a.md', files['docs/a.md'], { read, exists });
  assert.deepEqual(
    problems.map((p) => p.target),
    ['missing.md', 'b.md#nope', '#zzz'],
  );
});

const more = {
  'docs/c.md': '# Foo Bar\n<a id="MyId"></a>\n',
  'docs/my file.md': '# Spaced\n',
  'docs/page.md': '# Page\n',
};
const readMore = (p) => more[p];
const existsMore = (p) => p in more;
const check = (md) => checkFile('docs/x.md', md, { read: readMore, exists: existsMore }).map((p) => p.target);

test('a stray % is a broken link, not a crash', () => {
  assert.deepEqual(check('[a](c%zz.md) [b](c.md#100%)'), ['c%zz.md', 'c.md#100%']);
});

test('a ?query is ignored when the file is looked up', () => {
  assert.deepEqual(check('[a](c.md?plain=1#foo-bar) [b](missing.md?x=1)'), ['missing.md?x=1']);
});

test('anchors compare in lower case on both sides', () => {
  assert.deepEqual(check('[a](c.md#Foo-Bar) [b](c.md#myid) [c](c.md#MyId) [d](c.md#other)'), ['c.md#other']);
});

test('a badge linking an image checks the outer link and the image', () => {
  assert.deepEqual(check('[![b](gone.png)](page.md) [![c](page.md)](lost.md)'), ['gone.png', 'lost.md']);
});

test('footnote definitions are not links', () => {
  assert.deepEqual(check('text[^1]\n\n[^1]: see the paper\n'), []);
});

test('URL-encoded paths are decoded', () => {
  assert.deepEqual(check('[a](my%20file.md#spaced) [b](<my file.md>)'), []);
});

test('a .html link to a page that exists as .md is fine (the published site)', () => {
  assert.deepEqual(check('[a](page.html#page) [b](nothing.html) [c](page.html#nope)'), [
    'nothing.html',
    'page.html#nope',
  ]);
});
