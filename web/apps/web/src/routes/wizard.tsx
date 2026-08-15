import * as setupApi from "@loomarr/api/endpoints/setup";
import { SettingEntryProvenance } from "@loomarr/api/models/settingEntryProvenance";
import { unwrap } from "@loomarr/api/unwrap";
import { createFileRoute } from "@tanstack/react-router";
import { Loader2 } from "lucide-react";
import { useState } from "react";
import { useAuth } from "@/auth/use-auth";
import { WizardShell } from "@/components/loomarr/setup/wizard-shell";
import { useDocumentTitle } from "@/lib/use-document-title";
import { useSettingsEntries } from "@/settings/use-settings-entries";
import { BootstrapStep } from "@/wizard/bootstrap-step";
import { ChecklistStep } from "@/wizard/checklist-step";
import { FirstChannelStep } from "@/wizard/first-channel-step";
import { PlayoutStep } from "@/wizard/playout-step";
import {
  deriveStepStatuses,
  isConnectionId,
  isStepDone,
  PLAYOUT_INTERNAL,
  PLAYOUT_TUNARR,
  type PlayoutBackend,
  resolveStep,
  wizardSteps,
} from "@/wizard/steps";
import { TunarrLibraryStep } from "@/wizard/tunarr-library-step";
import { UsersStep } from "@/wizard/users-step";

// `?step=` and `?conn=` make the wizard deep-linkable (§13) — a support thread or a doc can
// point at "your Connections step, Tunarr block" instead of describing how to click there.
//
// ⚠ Both are REQUESTS, not state: the step is clamped against server truth (resolveStep) and
// the connection is narrowed to the Connections step's own sub-items. Neither can put the
// operator somewhere that does not exist or that they cannot leave.
//
// `skipped` deliberately stays component state. It is a record of THIS operator's choices,
// not a location — a shared link carrying it would assert someone else's decisions about
// what to pass on, and skipping is already recoverable by walking Back.
interface WizardSearch {
  step?: string;
  conn?: string;
}

// Operator first-run (§13, config-design §6). Public: step 1 (bootstrap) runs before any
// admin or session exists. Everything after is admin-gated, which the BE enforces —
// setup/status simply 401s until signed in. The wizard opens at the first incomplete
// step derived from server truth, so a refresh (or finishing from another browser)
// loses nothing; `visited` only lets the operator step back and forth within that.
const COPY: Record<string, { title: string; description: string }> = {
  bootstrap: {
    title: "Create your admin account",
    description: "This account owns Loomarr. You can set it up before connecting a media server.",
  },
  playout: {
    title: "How should Loomarr play your channels?",
    // Says what the answer CHANGES, because it changes the rest of the wizard. An operator
    // who does not know what Tunarr is should still be able to answer confidently.
    description: "This decides what you'll need to set up next. Most people should keep the default.",
  },
  checklist: {
    title: "Connect your services",
    description: "Loomarr live-tests each dependency. A red check tells you exactly what to fix.",
  },
  library: {
    title: "Give Tunarr your library",
    description: "Without this, Tunarr schedules slots that have no programme to play.",
  },
  users: {
    title: "Import media-server users",
    description:
      "Only the accounts you pick can sign in. You can skip this if you're the only one using Loomarr.",
  },
  channel: {
    title: "Your first channel",
    description: "Pick a starting point. You can edit it before Loomarr builds anything.",
  },
};

// A step the operator may pass on. Skipped reads neutral, never red (§6).
//
// The blocking set is "the shortest honest path to a live channel" (§13, config-design §6),
// which since §9.1 depends on the playout backend (see requiredChecks). A step that gates on
// a check the operator cannot satisfy is a dead end, because the wizard offers only
// Back/Continue and the rail is not clickable: they'd be stranded on that screen for good.
// It bit hardest on `library`, whose entire purpose (§6) is to stop channels scheduling slots
// with no programme, and hardest of all on `tunarr`, which an internal-playout install can
// never turn green. (Live TV is no longer a step, it auto-wires on the Tunarr save, so it
// can't strand anyone here either.)
const SKIPPABLE = new Set(["library", "users"]);

const WizardScreen = () => {
  useDocumentTitle("Setup");
  const { user, isAuthenticated, isLoading: authLoading } = useAuth();
  const status = setupApi.useSetupStatus({ query: { enabled: isAuthenticated, retry: false } });
  const checks = unwrap(status.data, (b) => b.checks) ?? [];

  // Who plays the channels (§9.1). Read from the registry rather than held in component
  // state, because it is a SETTING the operator may already have pinned via env or chosen on
  // a previous visit — and because it decides which steps exist, so a local copy would let
  // the rail and the resolver disagree.
  const entries = useSettingsEntries();
  const playoutEntry = entries.find((e) => e.key === "playout.backend");
  const backend: PlayoutBackend = playoutEntry?.value === PLAYOUT_TUNARR ? PLAYOUT_TUNARR : PLAYOUT_INTERNAL;
  // An env-pinned backend cannot be changed here (config-design §3); the step says so rather
  // than offering a control whose write would come back `pinned`.
  const playoutPinnedBy =
    playoutEntry?.provenance === SettingEntryProvenance.env
      ? (playoutEntry.envVar ?? "An environment variable")
      : undefined;
  const stepCtx = { checks, isAuthenticated, backend };

  const { step: requestedStep, conn: requestedConn } = Route.useSearch();
  const navigate = Route.useNavigate();

  const [skipped, setSkipped] = useState<ReadonlySet<string>>(new Set());

  // Which connection block is revealed on the Connections step. Media server is open on
  // arrival so the step never lands fully collapsed; picking the same one again closes it.
  // `conn` absent ⇒ that default; an unknown value narrows to it rather than opening nothing.
  const openConn = requestedConn ?? "media_server";
  // `replace` so toggling a block does not stack history entries — Back should leave the
  // wizard step, not undo five disclosure clicks.
  const toggleConn = (id: string) =>
    void navigate({
      search: (prev: WizardSearch) => ({ ...prev, conn: prev.conn === id ? undefined : id }),
      replace: true,
    });

  // Land on the right step the FIRST time. The resume point is derived from server truth,
  // so computing it before `me` / `setup/status` settle would briefly show the wrong step
  // and then yank the operator forward — hold the paint instead of flashing.
  if (authLoading || (isAuthenticated && status.isLoading)) {
    return (
      <main className="flex min-h-screen items-center justify-center bg-background" aria-busy="true">
        <Loader2 className="size-6 animate-spin text-muted-foreground" aria-label="Checking your setup" />
      </main>
    );
  }

  // Server truth decides; the URL only asks. A link past the frontier clamps back to it, so
  // no link can strand the operator on a step whose Continue can never enable.
  // Every derivation reads the SAME list, built once from the chosen backend, so the rail,
  // the resolver and the Back/Continue indices cannot disagree about which steps exist.
  const steps = wizardSteps(backend);
  const currentId = resolveStep(requestedStep, stepCtx);
  const statusById = deriveStepStatuses({ ...stepCtx, currentId, skipped });
  const index = steps.findIndex((s) => s.id === currentId);
  const step = steps[index];
  const copy = COPY[currentId];
  const checkFor = (name: string) => checks.find((c) => c.name === name);
  // Moving between steps is real navigation (Back should undo it), so this pushes rather
  // than replaces. `conn` is dropped on the way out: it means nothing off the Connections
  // step, and a stale one would silently reopen a block on the next visit.
  const goTo = (id: string | undefined) =>
    void navigate({ search: (prev: WizardSearch) => ({ ...prev, step: id, conn: undefined }) });

  const body = () => {
    switch (currentId) {
      case "bootstrap":
        // The owner is resolved HERE, not in the step: this route already read identity to
        // decide which step to show, so passing it down keeps one answer to one question.
        return <BootstrapStep onDone={() => goTo("playout")} ownerName={user?.name} />;
      case "playout":
        return <PlayoutStep value={backend} pinnedBy={playoutPinnedBy} />;
      case "checklist":
        return <ChecklistStep openId={openConn} onToggle={toggleConn} backend={backend} />;
      case "library":
        return <TunarrLibraryStep check={checkFor("tunarr_library")} />;
      case "users":
        return <UsersStep />;
      default:
        return <FirstChannelStep />;
    }
  };

  // Bootstrap advances ITSELF while it still has a form to submit (it owns "Create admin"),
  // and the final step finishes the wizard rather than advancing; everything between uses
  // the shell's Continue, gated on that step's server-derived completion.
  //
  // ⚠ Once an admin EXISTS, bootstrap has no form left — it renders a read-only confirmation
  // — so it must take the shell's Continue like any other done step. Without this clause an
  // operator who walked Back to it would find no form AND no Continue: a dead end created by
  // fixing the other one.
  const bootstrapSelfAdvances = currentId === "bootstrap" && !isAuthenticated;
  const advances = !bootstrapSelfAdvances && index < steps.length - 1;
  const skip = () => {
    setSkipped((prev) => new Set(prev).add(currentId));
    goTo(steps[index + 1]?.id);
  };

  return (
    <WizardShell
      steps={steps}
      currentId={currentId}
      statusById={statusById}
      // The rail's connection sub-items drive (and reflect) which block is revealed. Only
      // meaningful on the Connections step; other steps carry no subItems.
      activeSubItem={currentId === "checklist" ? openConn : undefined}
      onSubItem={toggleConn}
      title={copy?.title ?? step?.title ?? "Setup"}
      description={copy?.description}
      onBack={index > 0 ? () => goTo(steps[index - 1]?.id) : undefined}
      onNext={advances ? () => goTo(steps[index + 1]?.id) : undefined}
      onSkip={advances && SKIPPABLE.has(currentId) ? skip : undefined}
      // A skippable step has no server check to satisfy, so it must never BLOCK: gating
      // Continue on `isStepDone` there would strand an operator who did the optional work
      // (imported users) behind a button that can never enable. Skip stays, to record the
      // deliberate pass as `skipped` rather than merely unfinished.
      nextDisabled={advances && !SKIPPABLE.has(currentId) && !isStepDone(currentId, stepCtx)}
      busy={status.isFetching}
    >
      {body()}
    </WizardShell>
  );
};

const Route = createFileRoute("/wizard")({
  // `?step=` and `?conn=` are deep-linkable (§13). Both NARROW rather than validate-and-throw,
  // the same shape `?tab=` uses: a stale link from an older release lands on a real screen
  // instead of an error. The step is only sanity-checked for shape here — whether it is
  // REACHABLE depends on server truth the router does not have, so that clamp lives in
  // resolveStep, where the checks are.
  //
  // ⚠ Checked against EVERY step and connection either backend can have, not the chosen
  // one's: validateSearch runs before the component, so the settings query has not resolved
  // and the backend is unknown here. Narrowing to the internal path would silently drop
  // `?step=library` and `?conn=tunarr` on a Tunarr install, turning a valid deep link into
  // a fall-through. resolveStep does the backend-aware clamp once the answer is in hand.
  validateSearch: (search: Record<string, unknown>): WizardSearch => {
    const known = (id: unknown) =>
      [...wizardSteps(PLAYOUT_INTERNAL), ...wizardSteps(PLAYOUT_TUNARR)].some((s) => s.id === id);
    const knownConn = (id: string | undefined) =>
      isConnectionId(id, PLAYOUT_INTERNAL) || isConnectionId(id, PLAYOUT_TUNARR);
    return {
      step: known(search.step) ? (search.step as string) : undefined,
      conn: knownConn(search.conn as string | undefined) ? (search.conn as string) : undefined,
    };
  },
  component: WizardScreen,
});

export { Route };
