import * as libraryApi from "@loomarr/api/endpoints/library";
import * as settingsApi from "@loomarr/api/endpoints/settings";
import * as setupApi from "@loomarr/api/endpoints/setup";
import { LibraryStatusOutputBodyHealth } from "@loomarr/api/models/libraryStatusOutputBodyHealth";
import { SettingEntryProvenance } from "@loomarr/api/models/settingEntryProvenance";
import { SettingKey } from "@loomarr/api/models/settingKey";
import { unwrap } from "@loomarr/api/unwrap";
import { invalidateSettings } from "@loomarr/core/events";
import { LoadingStrip, Stack } from "@loomarr/design-system";
import { useQueryClient } from "@tanstack/react-query";
import { getRouteApi } from "@tanstack/react-router";
import { useState } from "react";
import { useAuth } from "@/auth/use-auth";
import { ErrorState } from "@/components/loomarr/feedback";
import { InstallationLocation } from "@/components/loomarr/settings/installation-location";
import { WizardShell } from "@/components/loomarr/setup/wizard-shell";
import { useDocumentTitle } from "@/lib/use-document-title";
import { useSettingsDraft } from "@/settings/settings-draft";
import { BootstrapStep } from "@/wizard/bootstrap-step";
import { ChecklistStep } from "@/wizard/checklist-step";
import { FirstChannelStep } from "@/wizard/first-channel-step";
import { LibraryStep } from "@/wizard/library-step";
import { PlayoutStep } from "@/wizard/playout-step";
import {
  deriveStepStatuses,
  isStepDone,
  PLAYOUT_INTERNAL,
  PLAYOUT_TUNARR,
  type PlayoutBackend,
  resolveStep,
  wizardSteps,
} from "@/wizard/steps";
import { TunarrLibraryStep } from "@/wizard/tunarr-library-step";
import { UsersStep } from "@/wizard/users-step";
import { COPY, continueGated, SKIPPABLE } from "../wizard-rules";

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
// loses nothing.
const route = getRouteApi("/wizard");

const WizardPage = () => {
  useDocumentTitle("Setup");
  const queryClient = useQueryClient();
  const { user, isAuthenticated, isLoading: authLoading } = useAuth();
  const status = setupApi.useSetupStatus({ query: { enabled: isAuthenticated, retry: false } });
  const checks = unwrap(status.data, (b) => b.checks) ?? [];

  // Who plays the channels (§9.1). Read from the registry rather than held in component
  // state, because it is a SETTING the operator may already have pinned via env or chosen on
  // a previous visit — and because it decides which steps exist, so a local copy would let
  // the rail and the resolver disagree.
  const settings = settingsApi.useSettingsList({ query: { enabled: isAuthenticated, retry: false } });
  const entries = unwrap(settings.data, (b) => b.settings) ?? [];
  const playoutEntry = entries.find((e) => e.key === SettingKey.PlayoutBackend);
  const publicURLEntry = entries.find((e) => e.key === SettingKey.ServerPublicURL);
  const locationEntries = entries.filter(
    (entry) =>
      entry.key === SettingKey.FillerHomeCountry ||
      entry.key === SettingKey.FillerHomePlace ||
      entry.key === SettingKey.FillerLocalRadiusKm ||
      entry.key === SettingKey.FillerLanguage,
  );
  const persisted = (key: SettingKey) => locationEntries.find((entry) => entry.key === key)?.value;
  const persistedCountry = persisted(SettingKey.FillerHomeCountry) ?? "";
  const {
    edits: locationEdits,
    setEdit: setLocationEdit,
    resetEdits: resetLocationEdits,
  } = useSettingsDraft();
  const liveCountry = locationEdits[SettingKey.FillerHomeCountry] ?? persistedCountry;
  const livePlace = locationEdits[SettingKey.FillerHomePlace] ?? persisted(SettingKey.FillerHomePlace) ?? "";
  const liveRadius =
    locationEdits[SettingKey.FillerLocalRadiusKm] ?? persisted(SettingKey.FillerLocalRadiusKm) ?? "";
  const liveLanguage =
    locationEdits[SettingKey.FillerLanguage] ?? persisted(SettingKey.FillerLanguage) ?? "en";
  const locationPatch = settingsApi.useSettingsPatch({
    mutation: {
      onSuccess: async () => {
        await invalidateSettings(queryClient);
      },
    },
  });
  const backend: PlayoutBackend = playoutEntry?.value === PLAYOUT_TUNARR ? PLAYOUT_TUNARR : PLAYOUT_INTERNAL;
  // An env-pinned backend cannot be changed here (config-design §3); the step says so rather
  // than offering a control whose write would come back `pinned`.
  const playoutPinnedBy =
    playoutEntry?.provenance === SettingEntryProvenance.env
      ? (playoutEntry.envVar ?? "An environment variable")
      : undefined;
  // The Library step is done once a folder is in use and Loomarr can open it; the import never
  // holds the wizard (#2151 decision 30). Shares its cache with the step's own reads.
  const library = libraryApi.useLibraryStatus({ query: { enabled: isAuthenticated, retry: false } });
  const stepCtx = {
    checks,
    isAuthenticated,
    backend,
    publicURL: publicURLEntry?.value,
    installationCountry: persistedCountry,
    libraryReady: unwrap(library.data)?.health === LibraryStatusOutputBodyHealth.ok,
  };

  const { step: requestedStep, conn: requestedConn } = route.useSearch();
  const navigate = route.useNavigate();

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
  if (authLoading || (isAuthenticated && (status.isLoading || settings.isLoading))) {
    return (
      <Stack
        align="center"
        backgroundColor="$surfaceCanvas"
        direction="row"
        justify="center"
        minHeight="100vh"
        render="main"
        aria-busy
      >
        <LoadingStrip accessibilityLabel="Checking your setup" />
      </Stack>
    );
  }

  // The registry decides the backend-shaped route AND whether the internal path is complete.
  // An empty list after a failed request would silently mean "Tunarr, unfinished", which can
  // route the operator to a screen that cannot recover. Surface the request failure and keep
  // retry local to this idempotent read instead.
  if (isAuthenticated && settings.error) {
    return (
      <Stack
        align="center"
        backgroundColor="$surfaceCanvas"
        direction="row"
        justify="center"
        minHeight="100vh"
        padding={24}
        render="main"
      >
        <ErrorState error={settings.error} onRetry={() => void settings.refetch()} />
      </Stack>
    );
  }

  // Server truth decides; the URL only asks. A link past the frontier clamps back to it, so
  // no link can strand the operator on a step whose Continue can never enable.
  // Every derivation reads the SAME list, built once from the chosen backend, so the rail,
  // the resolver and the Back/Continue indices cannot disagree about which steps exist.
  const steps = wizardSteps(backend);
  const currentId = resolveStep(requestedStep, stepCtx);
  const statusById = deriveStepStatuses({ ...stepCtx, currentId, skipped });
  // The rail may go wherever a `?step=` link may: the steps resolveStep would honour.
  const reachable = new Set(steps.filter((s) => resolveStep(s.id, stepCtx) === s.id).map((s) => s.id));
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
        // The owner is resolved HERE, not in the step: this page already reads identity to
        // decide which step to show, so passing it down keeps one answer to one question.
        return <BootstrapStep onDone={() => goTo("playout")} ownerName={user?.name} />;
      case "playout":
        return <PlayoutStep value={backend} pinnedBy={playoutPinnedBy} />;
      case "location":
        return (
          <InstallationLocation
            entries={locationEntries}
            values={{
              [SettingKey.FillerHomeCountry]: liveCountry,
              [SettingKey.FillerHomePlace]: livePlace,
              [SettingKey.FillerLocalRadiusKm]: liveRadius,
              [SettingKey.FillerLanguage]: liveLanguage,
            }}
            onChange={setLocationEdit}
            results={unwrap(locationPatch.data, (body) => body.results) ?? undefined}
            error={locationPatch.error?.detail}
            showHeading={false}
          />
        );
      case "checklist":
        return <ChecklistStep openId={openConn} onToggle={toggleConn} backend={backend} />;
      case "media":
        return <LibraryStep backend={backend} />;
      case "library":
        return <TunarrLibraryStep check={checkFor("tunarr_library")} />;
      case "users":
        return <UsersStep />;
      default:
        return <FirstChannelStep llmReady={checkFor("llm")?.ok ?? false} />;
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
  const advance = async () => {
    if (currentId !== "location") {
      goTo(steps[index + 1]?.id);
      return;
    }
    if (!liveCountry.trim()) return;
    if (Object.keys(locationEdits).length > 0) {
      const response = await locationPatch.mutateAsync({ data: { edits: locationEdits } });
      const results = unwrap(response, (body) => body.results) ?? [];
      if (
        !Object.keys(locationEdits).every((key) =>
          results.some((result) => result.key === key && result.status === "saved"),
        )
      ) {
        return;
      }
      resetLocationEdits();
    }
    goTo("checklist");
  };
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
      reachable={reachable}
      onStep={goTo}
      title={copy?.title ?? step?.title ?? "Setup"}
      description={copy?.description}
      onBack={index > 0 ? () => goTo(steps[index - 1]?.id) : undefined}
      onNext={advances ? () => void advance() : undefined}
      onSkip={advances && SKIPPABLE.has(currentId) ? skip : undefined}
      // A skippable step `continueGated` lets through must never BLOCK: gating
      // Continue on `isStepDone` there would strand an operator who did the optional work
      // (imported users) behind a button that can never enable. Skip stays, to record the
      // deliberate pass as `skipped` rather than merely unfinished.
      nextDisabled={
        advances &&
        continueGated(currentId) &&
        (currentId === "location"
          ? !liveCountry.trim() || locationPatch.isPending
          : !isStepDone(currentId, stepCtx))
      }
      busy={status.isFetching || locationPatch.isPending}
    >
      {body()}
    </WizardShell>
  );
};

export type { WizardSearch };
export { WizardPage };
