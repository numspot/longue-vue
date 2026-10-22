import { strict as assert } from 'node:assert';
import { createHash } from 'node:crypto';
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { test } from 'node:test';
import { gzipSync, gunzipSync } from 'node:zlib';
import { canonicalizeArchive, prepareDesignSystem, readPin, verifyArchive } from './prepare-design-system.mjs';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const pin = readPin(root);

function fixture(t, overrides = {}) {
  const directory = mkdtempSync(join(tmpdir(), 'longue-vue-ds-test-'));
  t.after(() => {
    // Only remove the exact isolated fixture created by this test.
    const absolute = resolve(directory);
    assert.equal(dirname(absolute), resolve(tmpdir()));
    assert.ok(absolute.startsWith(join(resolve(tmpdir()), 'longue-vue-ds-test-')));
    rmSync(absolute, { recursive: true, force: true });
  });
  writeFileSync(join(directory, 'design-system.lock.json'), JSON.stringify({ ...pin, ...overrides }));
  return directory;
}

test('source pin and package manifests point only to ignored generated output', () => {
  assert.match(pin.commit, /^[a-f0-9]{40}$/);
  const packageJSON = JSON.parse(readFileSync(join(root, 'package.json'), 'utf8'));
  const lock = JSON.parse(readFileSync(join(root, 'package-lock.json'), 'utf8'));
  const spec = `file:.generated/${pin.archive}`;
  assert.equal(packageJSON.dependencies['@cloud-console/numspot-ds'], spec);
  assert.equal(lock.packages[''].dependencies['@cloud-console/numspot-ds'], spec);
  assert.equal(lock.packages['node_modules/@cloud-console/numspot-ds'].resolved, spec);
  assert.match(readFileSync(join(root, '.gitignore'), 'utf8'), /^\/\.generated\/$/m);
});

test('a verified local cache does not need Git, npm, or private network access', (t) => {
  const contents = Buffer.from('synthetic bootstrap-test archive; not a design-system implementation');
  const sha256 = createHash('sha256').update(contents).digest('hex');
  const directory = fixture(t, { sha256 });
  mkdirSync(join(directory, '.generated'));
  const archive = join(directory, '.generated', pin.archive);
  writeFileSync(archive, contents);
  const actual = prepareDesignSystem(directory, { runCommand: () => assert.fail('Cache hit must not execute commands') });
  assert.equal(actual, archive);
});

test('a corrupted local cache is refused rather than trusted or installed', (t) => {
  const directory = fixture(t);
  mkdirSync(join(directory, '.generated'));
  writeFileSync(join(directory, '.generated', pin.archive), 'corrupted');
  assert.throws(() => prepareDesignSystem(directory), /checksum mismatch/);
});

test('an unexpected upstream commit is rejected before executing npm', (t) => {
  const directory = fixture(t);
  const commands = [];
  assert.throws(() => prepareDesignSystem(directory, {
    runCommand(command, args) {
      commands.push(command);
      return args[0] === 'rev-parse' ? '0'.repeat(40) : '';
    },
  }), /Unexpected upstream commit/);
  assert.deepEqual(commands, ['git', 'git']);
});

test('archive paths cannot escape the generated directory', (t) => {
  const directory = fixture(t, { archive: '../outside.tgz' });
  assert.throws(() => readPin(directory), /filename, not a path/);
});

test('canonical archive ignores gzip compression differences and round-trips block boundaries', () => {
  for (const size of [0, 1, 65535, 65536, 131070, 675840]) {
    const contents = Buffer.alloc(size, 'fixture');
    const first = canonicalizeArchive(gzipSync(contents, { level: 1 }));
    const second = canonicalizeArchive(gzipSync(contents, { level: 9 }));
    assert.deepEqual(first, second);
    assert.deepEqual(gunzipSync(first), contents);
    assert.equal(first[9], 255);
    assert.equal(first.readUInt32LE(first.length - 4), size);
  }
});

test('archive verification rejects unexpected output', (t) => {
  const directory = fixture(t);
  const archive = join(directory, 'candidate.tgz');
  writeFileSync(archive, 'not the pinned package');
  assert.throws(() => verifyArchive(archive, pin.sha256), /checksum mismatch/);
});
