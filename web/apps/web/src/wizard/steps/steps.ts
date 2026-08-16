import type { SetupCheck } from "@loomarr/api/models/setupCheck";
import type { WizardStep, WizardStepStatus } from "@/components/loomarr/setup/wizard-shell";

// PlayoutBackend is who actually streams a channel (design §9.1). `internal` is the default:
// Loomarr encodes and serves the stream itself, and the media server picks it up as Live TV.
type PlayoutBackend = "internal" | "tunarr";

const PLAYOUT_INTERNAL: PlayoutBackend = "internal";
const PLAYOUT_TUNARR: PlayoutBackend = "tunarr";

// ⚠ THE STEP LIST IS DERIVED, NOT CONSTANT (design §13, config-design §6).
//
// §9.1 made internal playout the default and the wizard had not caught up: `tunarr` was a
// hardcoded blocking check and had its own wiring step, so the DEFAULT path demanded a second
// server the install would never use, and refused to continue until it answered. That is not a
// copy problem. The shortest honest path to a live channel no longer runs through Tunarr, so a
// fixed list cannot describe it.
//
// Steps the choice removes are HIDDEN, not shown as satisfied: a rail entry reading "not
// needed" still advertises work, and on the internal path Tunarr wiring is not deferred or
// optional, it is not part of this install at all.
//
// There is NO standalone "TV guide" step on either path: saving the Tunarr connection
// auto-wires Live TV into the media server's guide (config-design §6), so a separate step
// would only re-run the same no-op — the `livetv` outcome surfaces on that connection instead.
const wizardSteps = (backend: PlayoutBackend): WizardStep[] => {
  const connections: WizardStep = {
    id: "checklist",
    title: "Connections",
    // The individual connections show as sub-items under this step; picking one reveals
    // its form in the content (§13). ids match the setup-check names.
    subItems: [
      { id: "media_server", label: "Media server" },
      // No Tunarr sub-item on either path: its form lives on the Playout step now, under the
      // choice that makes it relevant (design §13).
      { id: "requester", label: "Requester" },
      { id: "tmdb", label: "TMDB" },
      { id: "ai", label: "AI" },
    ],
  };
  return [
    { id: "bootstrap", title: "Admin" },
    { id: "playout", title: "Playout" },
    connections,
    // Wiring Tunarr's library exists only where Tunarr plays: internal playout reads the
    // library directly, so there is no equivalent to do.
    ...(backend === PLAYOUT_TUNARR ? [{ id: "library", title: "Library" }] : []),
    { id: "users", title: "Users", optional: true },
    { id: "channel", title: "First channel" },
  ];
};

// The default-path step list, for callers with no backend in hand (stories, the rail's
// static shape). Prefer wizardSteps(backend) anywhere the operator's choice is known.
const WIZARD_STEPS: WizardStep[] = wizardSteps(PLAYOUT_INTERNAL);

// The shortest honest path to a live channel (config-design §6 "defaults philosophy"), which
// since §9.1 depends on WHO IS STREAMING: the media server alone when Loomarr plays the
// channels, plus Tunarr when Tunarr does. Seerr/LLM/TMDB/filler are feature-gating, not
// blocking — they show in the checklist but never hold the wizard hostage.
//
// ⚠ A check that cannot be satisfied must never block. An internal install can never turn
// `tunarr` green, and the wizard offers only Back/Continue with a non-clickable rail, so
// requiring it stranded the operator on that screen with no way around it.
const requiredChecks = (backend: PlayoutBackend): string[] =>
  backend === PLAYOUT_TUNARR ? ["media_server", "tunarr"] : ["media_server"];

// The default-path blocking set, for callers with no backend in hand.
const REQUIRED_CHECKS = requiredChecks(PLAYOUT_INTERNAL);

// The wiring checks each own a later step, so the Connections step doesn't double-count
// them (§6 step→group mapping). `livetv` has no step of its own — it's auto-wired on the
// Tunarr save and reflected on that connection, not gated behind a wizard step.
const WIRING_CHECK_BY_STEP: Record<string, string> = {
  library: "tunarr_library",
};

const checkOk = (checks: SetupCheck[], name: string): boolean => checks.some((c) => c.name === name && c.ok);

// What every derivation below reads. `backend` defaults to internal when a caller has not
// resolved it yet (the settings query is still in flight), which matches the registry default
// and keeps the wizard on the shortest path rather than briefly demanding Tunarr.
interface StepContext {
  checks: SetupCheck[];
  isAuthenticated: boolean;
  backend?: PlayoutBackend;
  // The persisted, registry-normalized machine-client address. Internal playout cannot
  // publish a tuner until the media server has an absolute Loomarr URL to fetch from.
  publicURL?: string;
}

const backendOf = (ctx: StepContext): PlayoutBackend => ctx.backend ?? PLAYOUT_INTERNAL;

// Resume-safety lives here: completion is derived from server truth (GET /v1/setup/status
// + whether an admin session exists), never from client-side wizard progress. A refresh —
// or finishing setup from another browser — lands the operator in the right place.
const isStepDone = (id: string, ctx: StepContext): boolean => {
  if (id === "bootstrap") return ctx.isAuthenticated;
  // The registry always resolves a backend (internal by default), but that alone is not a
  // complete internal-playout answer: the media server also needs Loomarr's machine-reachable
  // address. Tunarr owns its own stream URLs, so that path needs no server.public_url here.
  if (id === "playout") {
    if (!ctx.isAuthenticated) return false;
    return backendOf(ctx) === PLAYOUT_TUNARR || (ctx.publicURL?.trim() ?? "") !== "";
  }
  if (id === "checklist") return requiredChecks(backendOf(ctx)).every((name) => checkOk(ctx.checks, name));
  const wiring = WIRING_CHECK_BY_STEP[id];
  return wiring ? checkOk(ctx.checks, wiring) : false;
};

const deriveStepStatuses = (
  ctx: StepContext & { currentId: string; skipped?: ReadonlySet<string> },
): Record<string, WizardStepStatus> => {
  const statuses: Record<string, WizardStepStatus> = {};
  // Iterates the DERIVED list, so a step the chosen backend removes gets no status at all
  // rather than a "pending" the rail would render as outstanding work.
  for (const step of wizardSteps(backendOf(ctx))) {
    if (step.id === ctx.currentId) statuses[step.id] = "current";
    else if (ctx.skipped?.has(step.id)) statuses[step.id] = "skipped";
    else if (isStepDone(step.id, ctx)) statuses[step.id] = "done";
    else statuses[step.id] = "pending";
  }
  return statuses;
};

// The first step that still needs the operator — where a resumed wizard opens.
const firstIncompleteStep = (ctx: StepContext): string =>
  wizardSteps(backendOf(ctx)).find((s) => !s.optional && !isStepDone(s.id, ctx))?.id ?? "channel";

// Where a `?step=` link actually lands (§13 deep links).
//
// ⚠ THE URL IS A REQUEST, NOT A LOCATION — server truth still decides. A link is written by
// whoever pastes it and read whenever someone clicks it, so it can easily name a step the
// operator cannot yet complete (`?step=users` before an admin account exists). Honouring it
// verbatim would STRAND them: the wizard offers only Back/Continue, the rail is not
// clickable, and Continue on an unmet required step is disabled — a screen with no way
// forward. This is the same reasoning SKIPPABLE encodes in the route.
//
// So a request beyond the frontier clamps to the frontier. Anything at or before it is
// honoured, which is what makes a link to an EARLIER step (the support case — "open your
// Connections step") work, and what lets the operator move back and forth.
//
// An unknown id falls through to the frontier too, so a stale link from an older release
// lands somewhere real rather than on a blank screen — the narrowing `?tab=` already uses.
// Resolved against the DERIVED list: a link naming a step this backend does not have
// (`?step=library` on an internal install) is as unreal as one naming a step that never
// existed, and falls through to the frontier the same way.
const resolveStep = (requested: string | undefined, ctx: StepContext): string => {
  const steps = wizardSteps(backendOf(ctx));
  const frontier = firstIncompleteStep(ctx);
  if (!requested) return frontier;
  const wanted = steps.findIndex((s) => s.id === requested);
  if (wanted === -1) return frontier;
  const limit = steps.findIndex((s) => s.id === frontier);
  return wanted <= limit ? requested : frontier;
};

// The connection sub-item a `?conn=` link may reveal (§13). Constrained to the Connections
// step's OWN sub-items so a link cannot open a block that does not exist on that step;
// anything else falls back to the default the step opens with.
//
// Backend-aware for the same reason: `?conn=tunarr` is a real block on the Tunarr path and
// nothing at all on the internal one, so a single static answer would be wrong on one of them.
const isConnectionId = (id: string | undefined, backend?: PlayoutBackend): boolean =>
  wizardSteps(backend ?? PLAYOUT_INTERNAL)
    .find((s) => s.id === "checklist")
    ?.subItems?.some((i) => i.id === id) ?? false;

export type { PlayoutBackend, StepContext };
export {
  deriveStepStatuses,
  firstIncompleteStep,
  isConnectionId,
  isStepDone,
  PLAYOUT_INTERNAL,
  PLAYOUT_TUNARR,
  REQUIRED_CHECKS,
  requiredChecks,
  resolveStep,
  WIZARD_STEPS,
  wizardSteps,
};
