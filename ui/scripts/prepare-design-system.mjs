// Fetch/build the private design system locally. Never commit its output.
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { copyFileSync, existsSync, mkdirSync, mkdtempSync, readFileSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { gunzipSync } from 'node:zlib';

export function readPin(uiRoot) {
  const pin = JSON.parse(readFileSync(join(uiRoot, 'design-system.lock.json'), 'utf8'));
  if (pin.archiveFormat !== 'canonical-gzip-store-v1') throw new Error('Unsupported design-system archive format.');
  if (!/^[a-f0-9]{40}$/.test(pin.commit) || !/^[a-f0-9]{64}$/.test(pin.sha256)) {
    throw new Error('The design-system source commit and archive checksum must be pinned.');
  }
  if (basename(pin.archive) !== pin.archive || !pin.archive.endsWith('.tgz')) {
    throw new Error('The generated archive must be a filename, not a path.');
  }
  return pin;
}

// npm's tar payload is reproducible, but Node/zlib gzip compression changes
// across releases. Emit RFC 1952 gzip with deterministic, uncompressed DEFLATE
// blocks. This small local-only package trades compression for portable hashes.
export function canonicalizeArchive(input) {
  const contents = gunzipSync(input);
  const blocks = Math.max(1, Math.ceil(contents.length / 65535));
  const output = Buffer.alloc(10 + contents.length + blocks * 5 + 8);
  Buffer.from([0x1f, 0x8b, 8, 0, 0, 0, 0, 0, 0, 255]).copy(output);
  let position = 10;
  for (let block = 0; block < blocks; block++) {
    const start = block * 65535;
    const size = Math.min(65535, contents.length - start);
    output[position++] = block === blocks - 1 ? 1 : 0;
    output.writeUInt16LE(size, position);
    output.writeUInt16LE(size ^ 0xffff, position + 2);
    position += 4;
    contents.copy(output, position, start, start + size);
    position += size;
  }
  let crc = 0xffffffff;
  for (const byte of contents) {
    crc ^= byte;
    for (let bit = 0; bit < 8; bit++) crc = (crc >>> 1) ^ ((crc & 1) ? 0xedb88320 : 0);
  }
  output.writeUInt32LE((crc ^ 0xffffffff) >>> 0, position);
  output.writeUInt32LE(contents.length >>> 0, position + 4);
  return output;
}

export function verifyArchive(archive, expected) {
  const actual = createHash('sha256').update(readFileSync(archive)).digest('hex');
  if (actual !== expected) {
    throw new Error(`Design-system checksum mismatch: expected ${expected}, got ${actual}. Review the source/toolchain; do not install this archive.`);
  }
}

export function prepareDesignSystem(uiRoot, { rebuild = false, runCommand } = {}) {
  const pin = readPin(uiRoot);
  const destination = join(uiRoot, '.generated');
  const archive = join(destination, pin.archive);
  if (!rebuild && existsSync(archive)) {
    verifyArchive(archive, pin.sha256);
    console.log(`NumSpot DS ${pin.version}: existing local package verified.`);
    return archive;
  }

  const checkout = mkdtempSync(join(tmpdir(), 'longue-vue-numspot-ds-'));
  const env = { ...process.env, HUSKY: '0', GIT_TERMINAL_PROMPT: '0' };
  const run = runCommand ?? ((command, args, capture = false) => execFileSync(command, args, {
    cwd: checkout, env, stdio: capture ? 'pipe' : 'inherit', encoding: 'utf8',
  }));

  try {
    run('git', ['clone', '--branch', pin.tag, '--depth', '1', pin.repository, '.']);
    const actualCommit = run('git', ['rev-parse', 'HEAD'], true).trim();
    if (actualCommit !== pin.commit) throw new Error(`Unexpected upstream commit: ${actualCommit}`);
    run('npm', ['ci', '--ignore-scripts', '--no-audit', '--no-fund']);

    const packagePath = join(checkout, 'package.json');
    const packageJSON = JSON.parse(readFileSync(packagePath, 'utf8'));
    packageJSON.version = pin.version;
    delete packageJSON.scripts.prepare;
    writeFileSync(packagePath, `${JSON.stringify(packageJSON, null, 2)}\n`);

    // Include generated DS utilities and font faces, not just token globals.
    const entryPath = join(checkout, 'src/index.ts');
    const entry = readFileSync(entryPath, 'utf8');
    if (!entry.startsWith("import './styles/globals.css'")) throw new Error('Upstream CSS entry changed; review packaging.');
    writeFileSync(entryPath, entry.replace("import './styles/globals.css'", "import './styles/index.css'"));
    run('npm', ['run', 'build']);

    // Vite library mode embeds fonts as data URLs. Restore relative assets so
    // the consuming build can keep longue-vue's existing font-src 'self' CSP.
    const cssPath = join(checkout, 'dist/styles.css');
    let css = readFileSync(cssPath, 'utf8');
    mkdirSync(join(checkout, 'dist/fonts'), { recursive: true });
    for (const name of ['RethinkSans-Regular.ttf', 'RethinkSans-SemiBold.ttf']) {
      const source = join(checkout, 'src/assets/fonts', name);
      const encoded = `data:font/ttf;base64,${readFileSync(source).toString('base64')}`;
      if (!css.includes(encoded)) throw new Error(`Upstream font packaging changed: ${name}`);
      css = css.replaceAll(encoded, `./fonts/${name}`);
      copyFileSync(source, join(checkout, 'dist/fonts', name));
    }
    writeFileSync(cssPath, css);
    mkdirSync(destination, { recursive: true });
    run('npm', ['pack', '--ignore-scripts', '--pack-destination', destination]);
    writeFileSync(archive, canonicalizeArchive(readFileSync(archive)));
    verifyArchive(archive, pin.sha256);
    console.log(`NumSpot DS ${pin.version}: generated and verified ${archive}`);
    return archive;
  } catch (error) {
    throw new Error(`Cannot prepare the private NumSpot design system. Ensure GitLab network/SSH access is available. ${error.message}`, { cause: error });
  } finally {
    // Deliberately retain the isolated checkout for build inspection/debugging.
    console.log(`Temporary design-system checkout: ${checkout}`);
  }
}

const uiRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    prepareDesignSystem(uiRoot, { rebuild: process.argv.includes('--rebuild') });
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
