# Security Policy

## Supported Versions

Image Drop is a desktop application distributed as pre-built binaries on the
[Releases page](https://github.com/frathe/imagedrop/releases). Only the
**latest release** is supported with security fixes; please update before
reporting an issue to confirm it's still reproducible.

## Reporting a Vulnerability

Please **do not** open a public GitHub issue for security vulnerabilities.

Instead, report it privately using one of these channels:

- [GitHub Security Advisories](https://github.com/frathe/imagedrop/security/advisories/new)
  for this repository (preferred)
- Email florianrathe@gmail.com

Please include:

- A description of the vulnerability and its potential impact
- Steps to reproduce, including the OS/platform and Image Drop version
- Any relevant logs, sample files, or proof-of-concept code

You should expect an initial response within a few days. If the issue is
confirmed, a fix will be prepared and a new release published; you'll be
credited in the release notes unless you'd prefer otherwise.

## Scope

This project is a local, offline image viewer — it doesn't run a network
service or process untrusted input beyond image files you choose to open.
Reports of particular interest include crashes or memory-safety issues
triggered by malformed image files (JPEG, PNG, GIF, WebP, BMP, TIFF, ICO,
XPM, HEIC/HEIF, AVIF) and issues in the HEIC/AVIF WASM decoders.
Dependency vulnerabilities are also welcome, though `make security` (see the
[README](../README.md)) already runs `govulncheck` and checks GitHub
Dependabot alerts as part of routine maintenance.
