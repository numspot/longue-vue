# Private NumSpot design-system builds

The public repository contains **integration code and build metadata only**.
NumSpot's proprietary design-system source, compiled package, fonts and generated
UI bundles must not be committed or uploaded to public caches/artifact stores.
The built UI and UI-enabled binaries/images remain subject to NumSpot's license;
not committing the package does not grant redistribution rights.

## Setup and validation

Prerequisites: Node 22.12+ (or a supported newer release), npm, Git, and authorized
network/Git access to the internal NumSpot GitLab repository.

From the project root:

```sh
make ui-install            # fetch/build/verify DS, then npm ci
make ui-check
make ui-test               # bootstrap unit tests + real UI unit tests
make ui-build              # generates the private UI bundle
make build                 # UI-enabled binary for authorized internal use
```

Equivalently, from the UI directory:

```sh
npm run setup
npm run test:setup
npm run typecheck
npm test
npx playwright install chromium
npm run test:e2e
```

The [setup script](./package.json) explicitly prepares the local package **before**
`npm ci`. An npm `preinstall` hook is too late to resolve a missing `file:`
dependency on a clean checkout. A warm generated package is reused only after its
checksum is verified; otherwise preparation fails rather than silently substituting
components. Do not put Git credentials in the repository, package URL or logs.

## Provenance and packaging

[design-system.lock.json](./design-system.lock.json) pins:

- Repository: internal `cloud-console/design-system`
- Tag: `v1.4.0`
- Commit: `d453905caa6fe72cc147a0c58775fb5667078a95`
- Package version: `1.4.0`
- Archive SHA-256: `9b8813b850ba9d4a99c8565cea4c77a4c1a9d37ca6847e6abb0b2ea949b280d6`

The [prepare script](./scripts/prepare-design-system.mjs) clones to an isolated
temporary directory, verifies the exact commit **before running npm**, builds the
package and writes it under `ui/.generated/`. That entire directory is Git-ignored.
Only its `file:` reference and integrity hash appear in the committed lockfile.
The temporary checkout is retained for inspection after the command finishes.

Packaging adjustments (components and design tokens are otherwise unchanged):

1. Set the upstream version placeholder to `1.4.0` and omit its build-only Husky
   `prepare` hook from the generated package.
2. Build upstream's complete CSS entry, including generated utilities and font
   faces, instead of its token-only library entry.
3. Restore the two font files from Vite library-mode data URLs to relative assets.
   The consumer then emits same-origin fonts under `/ui/assets/`, preserving the
   server's existing `font-src 'self'` policy without external font requests.
4. Normalize the npm tarball to deterministic, uncompressed gzip blocks
   (`canonical-gzip-store-v1`). npm's gzip output differs across Node/zlib
   versions even when package contents are identical; canonical packaging keeps
   lockfile integrity portable without weakening verification.

## Public CI and release boundary

**Public longue-vue binaries and images are backend-only (`noui`); `/ui/` returns
404.** Public GitHub workflows validate Go/codegen/OpenAPI/Helm and dependency
bootstrap logic, but **do not build or test the actual NumSpot UI**. Run the real
UI tests locally or in an authorized private pipeline. There are no fake DS
components or successful "skipped UI test" substitutes in public checks.

The public [Dockerfile](../Dockerfile) and
[GoReleaser configuration](../.goreleaser.yaml) exclude the UI. The root
[Docker ignore policy](../.dockerignore) also excludes private generated files,
including from `COPY . .` intermediate layers and exported build caches.

Authorized UI-enabled containers use a separate, explicitly internal build:

```sh
make docker-build-ui
# Uses Dockerfile.local and a generated local dependency; no private key/token
# is copied into the Docker build. Do not publish this image/cache publicly.
```

[Dockerfile.local.dockerignore](../Dockerfile.local.dockerignore) admits only the
pinned generated archive for that internal build. Never use public GitHub cache
or artifact uploads for npm caches, UI bundles, screenshots/traces, UI-enabled
Go build caches, binaries or container layers. Do not expose private credentials
or network access to untrusted pull-request code.

## Upgrading

Review a new upstream release, update its tag/commit/version/filename in the pin,
then force a fresh build:

```sh
cd ui
npm run design-system:prepare -- --rebuild
```

If the checksum changes, the script refuses to install the output and reports the
observed checksum. Review the source/toolchain/output before updating the trusted
hash. Update the dependency's generated filename in both npm manifests and the
internal Docker ignore exception, run setup and all UI checks again, and commit
**only metadata/integration changes**, never the generated archive or bundle.
