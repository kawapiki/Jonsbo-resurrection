# Versioned releases

Public repository: https://github.com/kawapiki/Jonsbo-resurrection

The `Release` workflow runs on `v*` tags. It validates semantic version syntax and the root `VERSION` file, runs Windows tests/vet, embeds the signature icon and version metadata, builds the GUI and console executables, creates an explicit-file-list ZIP with dependency notices/source, and publishes that ZIP plus its SHA-256 checksum to GitHub Releases. A version containing a hyphen is published as a prerelease. Existing releases are never overwritten automatically.

## Maintainer procedure

1. Update `VERSION` and add `docs/releases/<version>.md`. Run `scripts/build.ps1` and inspect `scripts/package.ps1 -Version <version>` output locally.
2. Verify physical displays, sensor readings, tray actions, normal/elevated startup and shutdown. Automated CI does not access USB or install drivers.
3. Submit the version and release notes through a PR. After required CI passes and the owner merges it into `master`, check out the merged commit, then create and push the matching annotated tag, for example `git tag -a v0.4.0 -m "Jonsbo Resurrection v0.4.0"` and `git push origin v0.4.0`. The tag must match `VERSION`; do not push changes directly to `master`.
4. Wait for the `Release` workflow to succeed, download the public ZIP, and verify its checksum and embedded version. Do not reuse a published version for changed binaries.

The local packaging script does not publish. It retains a unique staging directory for inspection and refuses to overwrite an existing archive. Only the tag-triggered workflow publishes; ordinary branch pushes run CI.

Never upload local `bin/`, `.tools/`, `research/`, app-data folders, or `config.local.json` wholesale. The archive contains no token, logs, captures, vendor DLL, or driver installer. Publish source matching the binaries, including the PawnIO signed module's matching source and license. The build-only resource compiler is pinned to `github.com/tc-hib/go-winres@v0.3.3`; application runtime has no third-party Go modules or CGO dependency.

Release binaries are currently unsigned. Signing should only be added with an authorized certificate and CI secret configuration; do not describe an unsigned binary as digitally signed. The separately distributed PawnIO driver/module signing is independent of application signing.
