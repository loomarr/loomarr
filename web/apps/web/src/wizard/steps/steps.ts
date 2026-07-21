import type { SetupCheck } from "@loomarr/api";
import type { WizardStep, WizardStepStatus } from "@/components/loomarr";

// The operator first-run path (§13, frontend-build-plan §5). The rail always shows all
// seven so the operator can see the whole road; steps 3–7 land in 13.3c.
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
    ],
  },
  { id: "guide", title: "TV guide" },
  { id: "webhooks", title: "Webhooks" },
  { id: "library", title: "Library" },
  { id: "users", title: "Users", optional: true },
  { id: "channel", title: "First channel" },
];

// The shortest honest path to a live channel is media server + Tunarr (config-design §6
// "defaults philosophy"). Seerr/LLM/TMDB/filler are feature-gating, not blocking — they
// show in the checklist but never hold the wizard hostage.
const REQUIRED_CHECKS = ["media_server", "tunarr"];

// The wiring checks each own a later step, so the Connections step doesn't double-count
// them (§6 step→group mapping).
const WIRING_CHECK_BY_STEP: Record<string, string> = {
  guide: "livetv",
  webhooks: "webhook",
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

export { deriveStepStatuses, firstIncompleteStep, isStepDone, REQUIRED_CHECKS, WIZARD_STEPS };
