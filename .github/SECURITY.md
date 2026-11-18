# Security policy

Stuarty handles people's footage, holds the credentials of their social accounts and AI providers, and is driven by AI agents, so security reports are welcome and taken seriously. The security model is [SDD-007](../docs/sdds/007-security-model.md), amended by [ADR-0032](../docs/adrs/0032-controlled-outbound-and-human-approval.md) and [ADR-0033](../docs/adrs/0033-secret-vault.md).

## Supported versions

The supported version is the latest minor of the current major; during 0.x it is the latest 0.x minor. Older versions are not supported. From 1.0 a security fix is a patch release and never carries a breaking change (see the compatibility promise in [CONTRIBUTING](CONTRIBUTING.md)), so staying on the latest minor of your major is always possible.

| Version | Supported |
| --- | --- |
| Latest minor of the current major (latest 0.x minor during 0.x) | yes |
| Latest minor of the previous major, for 90 days after the next major is released (from 2.0 on) | security fixes only |
| Any other minor, any older major, any pre-release tag | no |

## Reporting a vulnerability

Do not open a public issue. Use GitHub: Security, Report a vulnerability, at <https://github.com/AlexGalhardo/vukem-video/security/advisories/new>.

Include:

- the version (or commit);
- the operating system;
- the mode (local or network);
- the steps, or a sample file;
- the impact.

## What to expect

This is a single-maintainer project, so every time below is best effort.

- Acknowledgement within 3 business days.
- Assessment within 7 days.
- A fix for confirmed issues within 30 days, faster when critical.

## Disclosure policy

Coordinated disclosure, at most 90 days. The fix is prepared in a private fork, and the advisory and the release go out together. You are credited unless you decline.

## In scope

- Authentication and local-trust bypass.
- Host, Origin or CORS bypass.
- Scope or project-limit bypass.
- Path confinement bypass.
- Any outbound connection in a default run.
- FFmpeg invocation policy bypass (reading a non-media file, reaching a protocol).
- Injection (SQL, XSS, log).
- Archive or font handling flaws.
- Secrets exposure.
- Supply-chain weaknesses in our pipeline.

## Out of scope

- Vulnerabilities in FFmpeg, whisper.cpp or Go itself: report them upstream, and tell us too so that we can bump the pin.
- The residual risks documented in SDD-007 section 14: a process of the same user reading the token; no sandbox on Windows and macOS; the local-mode rule that any readable file can be imported.
- Denial of service that needs an authorised credential and stays within the documented limits.
- Missing hardening of the operator's own host.

## Safe harbour

Good-faith research on your own installation is welcome. Do not access other people's data.

## Verifying releases

Each release has `checksums.txt`, a keyless signature bundle `checksums.txt.sigstore.json`, an SPDX SBOM per archive and build provenance attestations. No long-lived signing key exists: the certificate is bound to the release workflow and the release tag.

```sh
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github\.com/AlexGalhardo/vukem-video/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
sha256sum --ignore-missing -c checksums.txt
gh attestation verify vukem-video_<version>_<os>_<arch>.tar.gz -R AlexGalhardo/vukem-video
```

On Windows PowerShell, compare `Get-FileHash -Algorithm SHA256` with the line in `checksums.txt`.

Container image, verified by digest with the same workflow identity:

```sh
cosign verify ghcr.io/alexgalhardo/vukem-video@sha256:<digest> \
  --certificate-identity-regexp '^https://github\.com/AlexGalhardo/vukem-video/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

## Hardening checklist for operators

- Network mode needs TLS.
- Keep FFmpeg updated.
- Set the import roots.
- Consider `VUKEM_VIDEO_SANDBOX=bwrap`.
- Run as a non-root user.
- Keep the workspace out of synced folders.
