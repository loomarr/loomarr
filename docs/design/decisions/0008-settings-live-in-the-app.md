# 0008. Settings live in the app; the environment pins

- **Date:** 2026-07-14
- **Status:** accepted
- **Supersedes:** "the wizard validates, does not store"

## Context

The first design kept every connection in environment variables, with a wizard that only validated
them. Fixing a wrong URL meant editing Compose, restarting and returning, and the operator faced five
external services before anything worked.

## Decision

Follow the \*arr convention: connections and integration settings are configured and stored in the app,
with a live test on each form. Any setting's environment variable pins it and wins over the stored
value, which prevents two silent sources of truth. Generated secrets are created at first boot and
regenerable in Settings.

## Consequences

- A first run needs no environment variables and boots straight into the wizard.
- The wizard is configure → validate → save, and the same checklist is the troubleshooting console
  afterwards.
- Secrets live in the database, so they are masked, never logged or echoed, and backups are secret
  material.
- The settings layering and registry are specified in `config-design.md`.
