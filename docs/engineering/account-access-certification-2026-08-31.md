# Account invitation and recovery certification — 2026-08-31

This record certifies the clean certification-only current-main v2 candidate for #705 through #716.
It covers the contact, invitation, notification, SMTP, sharing, activation, and local-recovery
slices together. The candidate is `test/account-invitation-certification-v2`, based on current
`origin/main`. Protected CI remains the merge gate; this record does not claim that the candidate
has merged or been released.

## Acceptance evidence

| #716 requirement | Authoritative evidence | Result |
| --- | --- | --- |
| Local email, copied-link, and QR invitation preserves the chosen role and stores an Argon2id verifier | `TestAccountAccessLifecycleCertification`; `TestInvitationRedemption_LocalCreatesArgon2idUserAndSession`; invitation-dialog component tests; `tests/e2e/invitation-join.spec.ts` | Passed. The combined test follows an emailed administrator invitation through activation, verifies the transferred contact and role, and authenticates the chosen password. Copy and QR present the same explicit-consent grant through the shared UI. |
| Imported activation pins the selected Library identity, chosen role, and first online proof | `TestInvitationRedemption_LibraryFailuresNeverFallbackOrConsumeInvitation`; `TestInvitationRedemption_LibraryProvesPinnedIdentityAndStoresOfflineVerifier`; `TestServicePinsAnExplicitEnabledLibraryAccount`; imported invitation component and visual states | Passed. Wrong identity, authoritative rejection, and provider outage leave the invitation pending. Offline readiness appears only after successful provider authentication. |
| Disabled or failed email leaves QR, copy, and direct-account paths usable | `TestPublishPersistsSuppressionWithoutCallingAnAdapter`; `TestEmailAdapterSendTestUsesAppliedConfigurationWithoutMaterializingADomainGrant`; invitation-dialog “email without making QR and copy depend on delivery” and clipboard-failure tests; direct local-create and bulk-import API tests | Passed. Sharing is independent of asynchronous delivery, and the People surface points failed delivery back to retry or QR/link sharing. |
| Expiry, regeneration, revocation, resend, duplicate submission, and concurrent redemption | Store conformance `Invitations/{RegenerateAndRevokeGrants,GrantPreviewRejectsEveryUnusableState,ConcurrentRedemption,SiblingGrantLifecycle,Retention}`; notification retry/idempotency tests; invitation service/API lifecycle tests | Passed on SQLite and Postgres. One transaction admits one person and session; every losing or unusable grant receives the closed not-found path. |
| Disabled person and recovery eligibility changes | Store conformance `PasswordRecovery/{DisabledAfterIssueIsUnusable,ConcurrentRedemption,NewRequestSupersedesOld,Retention}`; `TestPasswordRecoveryRequest_OnlyEligibleLocalPersonCreatesRecord`; session-disable negatives | Passed. Disabled, imported, unknown, contactless, and unverified identities create no public distinction and receive no Loomarr reset grant. |
| Recovery replaces the local credential and revokes sessions and sibling grants | `TestAccountAccessLifecycleCertification`; `TestPasswordRecoveryGrant_RedeemsArgon2idAndCannotBeReused`; recovery store conformance; `tests/e2e/password-recovery.spec.ts` | Passed. The old password and activation session fail after reset; the replacement Argon2id credential succeeds without changing the administrator role. |
| Secrets never cross durable or presentation boundaries | Combined lifecycle durable-row assertion; invitation/recovery ephemeral-worker tests; `TestBootSettingsRedactsSMTPPasswordFromApplicationLogs`; API problem-detail tests; account-action fragment/storage tests; invitation/recovery E2E storage assertions | Passed. Persistence contains SHA-256 hashes for machine bearers and Argon2id verifiers for human passwords. SMTP credentials, plaintext grants, passwords, and session tokens are absent from durable notification data and reviewed screenshots. |
| One conformance suite and deterministic external adapters | `make test-pg`; SQLite execution inside `make check`; shared `testkit.MediaServer`; SMTP protocol/classification tests; deterministic sequence sender in account-delivery tests | Passed. The same conformance assertions run for SQLite and Postgres; no unit or certification test uses a live Library or SMTP service. |
| Generated contracts and complete product gates | `make openapi-verify`; `make arch-docs-verify`; `make config-docs-verify`; `make check`; `make test-pg`; `make fe`; `make e2e`; `make fe-visual`; `make docs-lint`; `make retired-verify`; `make agent-verify BASE=origin/main` | Passed in the fresh integrated run. E2E passed 14/14, and visual/accessibility passed 1,194 checks with 2 intentional skips. |

## Manual visual review

The deterministic desktop/mobile gallery was regenerated and then rerun without updates. The
following representative baselines were inspected at rendered size rather than accepted only from
pixel equality:

- `people-userspage--invite-open-desktop-linux.png` — the local/Library choice, optional contact,
  initial role, and email independence are readable in one focused dialog.
- `people-invitationdialog--ready-mobile-linux.png` — the QR remains scannable, the copied-link
  alternative is explicit, destructive regeneration/revocation is separated, and controls retain
  practical mobile targets.
- `people-invitation-join--local-desktop-linux.png` — reserved identity, role, expiry, and explicit
  activation precede local credential input.
- `people-invitation-join--library-admin-mobile-linux.png` — provider ownership and the elevated
  administrator consequence remain visible without clipping at the mobile viewport.
- `people-invitationroster--delivery-states-desktop-linux.png` — queued, delivered, and failed email
  outcomes have distinct actions; failure copy preserves the QR/link fallback.
- `setup-password-recovery--reset-mobile-linux.png` — expiry, both password fields, and the explicit
  reset action remain readable and reachable on mobile.

The full `make fe-visual` run also covers axe, forced-colors, dialog focus return/trapping, and mobile
touch-target assertions for the invitation workflow.

## Gate results

- Focused race certification: passed (`TestAccountAccessLifecycleCertification`).
- `make check`: passed, including race-enabled Go tests and repository policy checks.
- `make test-pg`: passed for `internal/store`, `internal/backendtransition`, and `internal/app`.
- `make agent-verify BASE=origin/main`: passed.
- `make openapi-verify`, `make arch-docs-verify`, and `make config-docs-verify`: passed.
- `make docs-lint` and `make retired-verify`: passed.
- `make fe`: passed, including the production SPA and Storybook.
- `make e2e`: passed, 14/14.
- `make fe-visual`: passed, 1,194 visual/accessibility checks with 2 intentional skips.
- OpenAPI, architecture, and configuration generators reproduced byte-identical output.
