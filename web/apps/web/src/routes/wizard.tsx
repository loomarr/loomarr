import { setupApi } from "@loomarr/api";
import { createFileRoute } from "@tanstack/react-router";
import { Loader2 } from "lucide-react";
import { useState } from "react";
import { useAuth } from "@/auth";
import { WizardShell } from "@/components/loomarr";
import {
  BootstrapStep,
  ChecklistStep,
  deriveStepStatuses,
  FirstChannelStep,
  firstIncompleteStep,
  isStepDone,
  LiveTvStep,
  TunarrLibraryStep,
  UsersStep,
  WebhookStep,
  WIZARD_STEPS,
} from "@/wizard";

// Operator first-run (§13, config-design §6). Public: step 1 (bootstrap) runs before any
// admin or session exists. Everything after is admin-gated, which the BE enforces —
// setup/status simply 401s until signed in. The wizard opens at the first incomplete
// step derived from server truth, so a refresh (or finishing from another browser)
// loses nothing; `visited` only lets the operator step back and forth within that.
const COPY: Record<string, { title: string; description: string }> = {
  bootstrap: {
    title: "Create the owning admin",
    description: "This account owns the instance. It works with zero media-server config.",
  },
  checklist: {
    title: "Connect your services",
    description: "Loomarr live-tests each dependency. A red check tells you exactly what to fix.",
  },
  guide: {
    title: "Put your channels in the TV guide",
    description: "The step that makes Loomarr show up on the family's TV, not just in this app.",
  },
  webhooks: {
    title: "Tell Sonarr and Radarr where to report",
    description: "So Loomarr knows the moment a download lands, instead of polling and guessing.",
  },
  library: {
    title: "Give Tunarr your library",
    description: "Without this, channels schedule slots that have no program to play.",
  },
  users: {
    title: "Import media-server users",
    description: "Only the accounts you pick can sign in. Skippable — a solo install needs no one else.",
  },
  channel: {
    title: "Your first channel",
    description: "Pick a starting point. You can edit it before Loomarr builds anything.",
  },
};

// A step the operator may pass on. Skipped reads neutral, never red (§6).
//
// Every WIRING step is here, not just webhooks. The blocking set is media_server +
// tunarr (§13, config-design §6) — "the shortest honest path to a live channel" — and a
// step that gates on a check the operator cannot satisfy is a dead end, because the
// wizard offers only Back/Continue and the rail is not clickable: they are stranded on
// that screen for good. An install with no *arr apps can never turn `webhook` green, and
// one that doesn't use its media server's Live TV can never turn `livetv` green.
//
// It bit hardest on `library`, whose entire purpose (§6) is to stop channels scheduling
// slots with no program: gating it behind Live TV meant an operator whose guide wiring
// failed could never reach the guard against dead-air channels. Found by the maintainer
// smoke, which could not reach step 5 at all.
const SKIPPABLE = new Set(["guide", "webhooks", "library", "users"]);

const WizardScreen = () => {
  const { isAuthenticated, isLoading: authLoading } = useAuth();
  const status = setupApi.useSetupStatus({ query: { enabled: isAuthenticated, retry: false } });
  const checks = status.data?.status === 200 ? (status.data.data.checks ?? []) : [];

  const [visited, setVisited] = useState<string | undefined>();
  const [skipped, setSkipped] = useState<ReadonlySet<string>>(new Set());

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

  const currentId = visited ?? firstIncompleteStep({ checks, isAuthenticated });
  const statusById = deriveStepStatuses({ checks, isAuthenticated, currentId, skipped });
  const index = WIZARD_STEPS.findIndex((s) => s.id === currentId);
  const step = WIZARD_STEPS[index];
  const copy = COPY[currentId];
  const checkFor = (name: string) => checks.find((c) => c.name === name);
  const goTo = (id: string | undefined) => setVisited(id);

  const body = () => {
    switch (currentId) {
      case "bootstrap":
        return <BootstrapStep onDone={() => goTo("checklist")} />;
      case "checklist":
        return <ChecklistStep />;
      case "guide":
        return <LiveTvStep check={checkFor("livetv")} />;
      case "webhooks":
        return <WebhookStep />;
      case "library":
        return <TunarrLibraryStep check={checkFor("tunarr_library")} />;
      case "users":
        return <UsersStep />;
      default:
        return <FirstChannelStep />;
    }
  };

  // Bootstrap advances itself (it owns its "Create admin" submit) and the final step
  // finishes the wizard rather than advancing; everything between uses the shell's
  // Continue, gated on that step's server-derived completion.
  const advances = currentId !== "bootstrap" && index < WIZARD_STEPS.length - 1;
  const skip = () => {
    setSkipped((prev) => new Set(prev).add(currentId));
    goTo(WIZARD_STEPS[index + 1]?.id);
  };

  return (
    <WizardShell
      steps={WIZARD_STEPS}
      currentId={currentId}
      statusById={statusById}
      title={copy?.title ?? step?.title ?? "Setup"}
      description={copy?.description}
      onBack={index > 0 ? () => goTo(WIZARD_STEPS[index - 1]?.id) : undefined}
      onNext={advances ? () => goTo(WIZARD_STEPS[index + 1]?.id) : undefined}
      onSkip={advances && SKIPPABLE.has(currentId) ? skip : undefined}
      // A skippable step has no server check to satisfy, so it must never BLOCK: gating
      // Continue on `isStepDone` there would strand an operator who did the optional work
      // (imported users) behind a button that can never enable. Skip stays, to record the
      // deliberate pass as `skipped` rather than merely unfinished.
      nextDisabled={
        advances && !SKIPPABLE.has(currentId) && !isStepDone(currentId, { checks, isAuthenticated })
      }
      busy={status.isFetching}
    >
      {body()}
    </WizardShell>
  );
};

const Route = createFileRoute("/wizard")({
  component: WizardScreen,
});

export { Route };
