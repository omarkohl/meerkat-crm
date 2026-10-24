import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import {
  root,
  parseSnippet,
  parseLocations,
  checkSnippet,
  checkConfig,
  checkAll,
  loadSnippetHeaders,
  REQUIRED_HEADERS,
  INCLUDE_LINE,
} from './security-headers.mjs';

const read = (p) => readFileSync(join(root, p), 'utf8');

test('both deployment shapes send the same security headers', () => {
  assert.deepEqual(checkAll(), []);
});

test('the snippet defines every required header once', () => {
  const names = loadSnippetHeaders().map(([n]) => n);
  for (const h of REQUIRED_HEADERS) assert.equal(names.filter((n) => n === h).length, 1, h);
});

test('the two nginx configs differ only in the upstream', () => {
  const norm = (t) =>
    t
      .replace(/#[^\n]*\n/g, '')
      .replace(/http:\/\/(backend:8080|127\.0\.0\.1:8081)/g, 'http://UPSTREAM')
      .replace(/\s+/g, ' ')
      .trim();
  assert.equal(norm(read('frontend/nginx.conf')), norm(read('docker/nginx.conf')));
});

test('parseLocations finds nested blocks and bodies', () => {
  const locs = parseLocations(
    'server { location / { try_files $uri /index.html; } location ^~ /api/ { proxy_pass http://b; } }',
  );
  assert.deepEqual(
    locs.map((l) => l.path),
    ['/', '^~ /api/'],
  );
  assert.match(locs[1].body, /proxy_pass/);
});

test('parseSnippet reads quoted values and variables, ignoring comments', () => {
  assert.deepEqual(parseSnippet('# add_header X "no" always;\nadd_header A "b c" always;\nadd_header S $v always;'), [
    ['A', 'b c'],
    ['S', '$v'],
  ]);
});

test('checkSnippet flags a missing header and unsafe script-src', () => {
  const good = read('frontend/security-headers.conf');
  assert.deepEqual(checkSnippet(good), []);
  assert.ok(
    checkSnippet(good.replace(/add_header Permissions-Policy[^\n]*\n/, '')).some((p) =>
      p.includes('Permissions-Policy'),
    ),
  );
  assert.ok(
    checkSnippet(
      good.replace("default-src 'self'; script-src 'self'", "default-src 'self'; script-src 'self' 'unsafe-inline'"),
    ).some((p) => p.includes('unsafe')),
  );
  assert.ok(checkSnippet(good.replace("; frame-ancestors 'none'\"", '"')).some((p) => p.includes('frame-ancestors')));
});

test('checkConfig flags stray add_header, headerless static locations and headers on proxies', () => {
  const conf = read('frontend/nginx.conf');
  assert.deepEqual(checkConfig('x', conf), []);
  // Server-level header (nginx would also add it to proxied API responses)
  assert.ok(
    checkConfig('x', conf.replace('server {', 'server {\n add_header X-Frame-Options "DENY" always;')).some((p) =>
      p.includes('outside the snippet'),
    ),
  );
  // A static location that forgot the include
  assert.ok(checkConfig('x', conf.replace(INCLUDE_LINE, '')).some((p) => p.includes('without the security headers')));
  // The include on a proxied location would duplicate the backend's headers
  assert.ok(
    checkConfig(
      'x',
      conf.replace('proxy_pass http://backend:8080/api/;', `proxy_pass http://backend:8080/api/;\n ${INCLUDE_LINE}`),
    ).some((p) => p.includes('must not add headers')),
  );
});

test('both nginx configs let image uploads through the API proxy @023-US2', () => {
  for (const p of ['frontend/nginx.conf', 'docker/nginx.conf']) {
    const api = parseLocations(read(p)).find((l) => l.path.includes('/api/'));
    assert.match(api.body, /client_max_body_size\s+26m;/, p);
  }
});
