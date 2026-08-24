# Public repository boundary

Status: build contract  
Last updated: 2026-08-23

This repository contains the complete installed client and service, collection behavior, public protocols and adapters, dashboard/backend application code, installers, tests, fixtures, and release workflows.

Private cloud operations belong in a separate access-controlled repository. That boundary may contain deployment account identifiers, production infrastructure state, private runbooks, internal incident records, customer data, abuse-detection details, and production secrets. It must not redefine the public HTTP protocol or hide collection behavior.

Public examples use synthetic identifiers and data. Production credentials, private customer data, raw source/diffs, Git objects, transcripts, prompts, environment values, raw command output, and internal incident details are prohibited here. Security reports use the private channel documented in `SECURITY.md`.

The repository is not ready for public launch until the owner approves a license and corresponding `NOTICE` content. Until then, external contributions and release publication remain disabled.
