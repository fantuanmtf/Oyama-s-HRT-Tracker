# Security policy

## Supported versions

`0.1.x` is an experimental pre-release line. Security fixes are applied on a
best-effort basis; there is no long-term support commitment and no
production-support claim.

## Reporting a vulnerability

Do not open a public issue with exploit details, real health data, private
keys, passphrases, tokens, or production backups.

Preferred: use GitHub's private vulnerability reporting for this repository
(**Security → Report a vulnerability**) if it is available.

If private reporting is not available, open a minimal public issue asking the
maintainer for a private channel. Do not include the vulnerability details or
sensitive material in that issue.

Include, once a private channel is established:

- affected version/commit and platform;
- a synthetic reproducer;
- expected and observed behaviour;
- impact assessment;
- whether the issue affects the Go CLI, the container/bundle parser, the
  OpenPGP integration, the replay state, the packaging, or documentation.

## Scope

In scope:

- `hrt-cold` command behaviour and failure paths;
- `.hrtb` container and complete-backup envelope validation;
- sign/encrypt/decrypt/verify and pinned-signer behaviour;
- replay state handling and file publication;
- release packaging and signatures.

Out of scope:

- the Oyama HRT Tracker website or its Workers deployment;
- vulnerabilities in the linked OpenPGP libraries themselves (report upstream,
  and tell us if the impact reaches this tool);
- hosting, DNS, or GitHub account security;
- findings that require real health data, real keys, or a compromised endpoint
  to demonstrate.

## Research rules

- Use synthetic data only.
- Do not test against other people's accounts, backups, or devices.
- Do not perform denial-of-service testing against shared infrastructure.
- Do not publish exploit details before a fix or coordinated disclosure.
- No bug bounty is offered.

## No audit claim

This project is an engineering checkpoint with regression tests. It is not an
independent security audit, and the absence of a known finding does not mean
"no vulnerabilities".
