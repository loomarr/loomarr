import type { SetupCheck } from "@loomarr/api";
import type { WizardStep, WizardStepStatus } from "@/components/loomarr";

// The operator first-run path (§13, frontend-build-plan §5). The rail shows the whole road
// so the operator sees what's left. There is NO standalone "TV guide" step: saving the
// Tunarr connection auto-wires Live TV into the media server's guide (config-design §6),
// so a separate step would only re-run the same no-op — the `livetv` outcome surfaces on
// the Tunarr connection's own verdict instead.
const WIZARD_STEPS: WizardStep[] = [
  { id: "bootstrap", title: "Admin" },
  {
    id: "checklist",
    title: "Connections",
    // The individual connections show as sub-items under this step; picking one reveals
    // its form in the content (§13). ids match the setup-check names.
    subItems: [
      { id: "media_server", label: "Media server" },
      { id: "tunarr", label: "Tunarr" },
      { id: "requester", label: "Requester" },
      { id: "tmdb", label: "TMDB" },
      { id: "ai", label: "AI" },
    ],
  },
  { id: "library", title: "Library" },
  { id: "users", title: "Users", optional: true },
  { id: "channel", title: "First channel" },
];

// The shortest honest path to a live channel is media server + Tunarr (config-design §6
// "defaults philosophy"). Seerr/LLM/TMDB/filler are feature-gating, not blocking — they
// show in the checklist but never hold the wizard hostage.
const REQUIRED_CHECKS = ["media_server", "tunarr"];

// The wiring checks each own a later step, so the Connections step doesn't double-count
// them (§6 step→group mapping). `livetv` has no step of its own — it's auto-wired on the
// Tunarr save and reflected on that connection, not gated behind a wizard step.
const WIRING_CHECK_BY_STEP: Record<string, string> = {
  library: "tunarr_library",
};

const checkOk = (checks: SetupCheck[], name: string): boolean => checks.some((c) => c.name === name && c.ok);

// Resume-safety lives here: completion is derived from server truth (GET /v1/setup/status
// + whether an admin session exists), never from client-side wizard progress. A refresh —
// or finishing setup from another browser — lands the operator in the right place.
const isStepDone = (id: string, ctx: { checks: SetupCheck[]; isAuthenticated: boolean }): boolean => {
  if (id === "bootstrap") return ctx.isAuthenticated;
  if (id === "checklist") return REQUIRED_CHECKS.every((name) => checkOk(ctx.checks, name));
  const wiring = WIRING_CHECK_BY_STEP[id];
  return wiring ? checkOk(ctx.checks, wiring) : false;
};

const deriveStepStatuses = (ctx: {
  checks: SetupCheck[];
  isAuthenticated: boolean;
  currentId: string;
  skipped?: ReadonlySet<string>;
}): Record<string, WizardStepStatus> => {
  const statuses: Record<string, WizardStepStatus> = {};
  for (const step of WIZARD_STEPS) {
    if (step.id === ctx.currentId) statuses[step.id] = "current";
    else if (ctx.skipped?.has(step.id)) statuses[step.id] = "skipped";
    else if (isStepDone(step.id, ctx)) statuses[step.id] = "done";
    else statuses[step.id] = "pending";
  }
  return statuses;
};

// The first step that still needs the operator — where a resumed wizard opens.
const firstIncompleteStep = (ctx: { checks: SetupCheck[]; isAuthenticated: boolean }): string =>
  WIZARD_STEPS.find((s) => !s.optional && !isStepDone(s.id, ctx))?.id ?? "channel";

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
const resolveStep = (
  requested: string | undefined,
  ctx: { checks: SetupCheck[]; isAuthenticated: boolean },
): string => {
  const frontier = firstIncompleteStep(ctx);
  if (!requested) return frontier;
  const wanted = WIZARD_STEPS.findIndex((s) => s.id === requested);
  if (wanted === -1) return frontier;
  const limit = WIZARD_STEPS.findIndex((s) => s.id === frontier);
  return wanted <= limit ? requested : frontier;
};

// The connection sub-item a `?conn=` link may reveal (§13). Constrained to the Connections
// step's OWN sub-items so a link cannot open a block that does not exist on that step;
// anything else falls back to the default the step opens with.
const isConnectionId = (id: string | undefined): boolean =>
  WIZARD_STEPS.find((s) => s.id === "checklist")?.subItems?.some((i) => i.id === id) ?? false;

export {
  deriveStepStatuses,
  firstIncompleteStep,
  isConnectionId,
  isStepDone,
  REQUIRED_CHECKS,
  resolveStep,
  WIZARD_STEPS,
};
