# Public repository boundary

Status: source-publication boundary; adopted for public release by ADR-071
Last updated: 2026-09-04

This repository contains the complete installed client and service, collection behavior, public protocols and adapters, dashboard/backend application code, installers, tests, fixtures, and release workflows.

Private cloud operations belong in a separate access-controlled repository. That boundary may contain deployment account identifiers, production infrastructure state, private runbooks, internal incident records, customer data, abuse-detection details, and production secrets. It must not redefine the public HTTP protocol or hide collection behavior.

Public examples use synthetic identifiers and data. Production credentials, private customer data, raw source/diffs, Git objects, transcripts, prompts, environment values, raw command output, and internal incident details are prohibited here. Security reports use the private channel documented in `SECURITY.md`.

The public repository is licensed under Apache-2.0 with the attribution in `NOTICE`. Public launch still requires an operational private security-reporting and conduct-enforcement channel; do not publish a placeholder mailbox that is not monitored.

ADR-071 makes the source repository, including the Convex backend, public under Apache-2.0; that does not relax this file's separation of source from private cloud operations or permit secrets or customer data in the source repository.
