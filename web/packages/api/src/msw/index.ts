// Barrel for the generated MSW handlers (`@loomarr/api/msw`) — the frontend's shared mock layer,
// which is what `internal/testkit` is for the Go side ("Phases do not invent private mocks").
//
// What is generated here is the WIRING: the URL, the method, the status. That is the half worth
// generating — a renamed route (`/v1/suggestions` → `/v1/proposals`, V41) is fixed by a
// regenerate, where a hand-written path would silently stop matching and its test would keep
// passing against nothing.
//
// ⚠ THE DEFAULT DATA IS NOT TRUSTWORTHY. Optional fields generate as
// `arrayElement([value, undefined])`, so presence varies per CALL and nothing is seeded — a
// handler left on its defaults is FLAKY, not merely arbitrary. Always pass an override; fixtures
// live in `apps/web/src/test/fixtures` and are parsed through their generated zod schema.
//
// ⚠ Flat, like the zod barrel and unlike the endpoint one: handler names are operation-scoped
// (`getSettingsPatchMockHandler`) and verified collision-free. Hand-written, therefore guarded —
// see barrel.test.ts.
export * from "../../generated/endpoints/auth/auth.msw";
export * from "../../generated/endpoints/channels/channels.msw";
export * from "../../generated/endpoints/dashboard/dashboard.msw";
export * from "../../generated/endpoints/events/events.msw";
export * from "../../generated/endpoints/filler/filler.msw";
export * from "../../generated/endpoints/help/help.msw";
export * from "../../generated/endpoints/images/images.msw";
export * from "../../generated/endpoints/jobs/jobs.msw";
export * from "../../generated/endpoints/library/library.msw";
export * from "../../generated/endpoints/ops/ops.msw";
export * from "../../generated/endpoints/playout/playout.msw";
export * from "../../generated/endpoints/proposals/proposals.msw";
export * from "../../generated/endpoints/search/search.msw";
export * from "../../generated/endpoints/settings/settings.msw";
export * from "../../generated/endpoints/setup/setup.msw";
export * from "../../generated/endpoints/system/system.msw";
export * from "../../generated/endpoints/titles/titles.msw";
export * from "../../generated/endpoints/users/users.msw";
