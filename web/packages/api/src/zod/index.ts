// Barrel for the generated zod schemas (`@loomarr/api/zod`).
//
// These exist so hand-written validation can DERIVE the wire's field names instead of
// mirroring them. `intentSchema` in packages/core once said `maxAcquire` where the wire says
// `maxAcquisitions`, and `runtimeTarget` where it says `runtimeTargetMin`; both parsed fine,
// both serialized to JSON the server ignored, and a user's acquisition cap silently vanished.
// Composing a form schema off one of these makes a wrong name a COMPILE error (§14).
//
// ⚠ FLAT, where the endpoint barrel is namespaced — the difference is deliberate. Namespacing
// exists there because orval repeats its fetch-client helper enums in every tag file and a flat
// re-export would collide. Zod names are operation-scoped (`submitProposalBody`, `loginBody`)
// and verified collision-free; if a future spec ever does collide, a duplicate export is a loud
// build error, not a silent shadow.
//
// ⚠ Hand-written, therefore guarded — see barrel.test.ts. A generated module nobody can import
// is indistinguishable from one that was never built.
export * from "../../generated/zod/auth/auth.zod";
export * from "../../generated/zod/channels/channels.zod";
export * from "../../generated/zod/dashboard/dashboard.zod";
export * from "../../generated/zod/filler/filler.zod";
export * from "../../generated/zod/help/help.zod";
export * from "../../generated/zod/images/images.zod";
export * from "../../generated/zod/jobs/jobs.zod";
export * from "../../generated/zod/library/library.zod";
export * from "../../generated/zod/ops/ops.zod";
export * from "../../generated/zod/playout/playout.zod";
export * from "../../generated/zod/proposals/proposals.zod";
export * from "../../generated/zod/search/search.zod";
export * from "../../generated/zod/settings/settings.zod";
export * from "../../generated/zod/setup/setup.zod";
export * from "../../generated/zod/system/system.zod";
export * from "../../generated/zod/titles/titles.zod";
export * from "../../generated/zod/users/users.zod";
