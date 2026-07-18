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
  firstIncompleteStep,
  isStepDone,
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
};

const WizardScreen = () => {
  const { isAuthenticated, isLoading: authLoading } = useAuth();
  const status = setupApi.useSetupStatus({ query: { enabled: isAuthenticated, retry: false } });
  const checks = status.data?.status === 200 ? (status.data.data.checks ?? []) : [];

  const [visited, setVisited] = useState<string | undefined>();

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
  const statusById = deriveStepStatuses({ checks, isAuthenticated, currentId });
  const index = WIZARD_STEPS.findIndex((s) => s.id === currentId);
  const step = WIZARD_STEPS[index];
  const copy = COPY[currentId];

  // Steps 3–7 land in 13.3c; the rail already shows the whole road so the operator can
  // see what's left rather than being surprised by it.
  const body = () => {
    if (currentId === "bootstrap") return <BootstrapStep onDone={() => setVisited("checklist")} />;
    if (currentId === "checklist") return <ChecklistStep />;
    return (
      <p className="text-muted-foreground text-sm">
        This step lands in the next release — the rail shows where it fits in the setup.
      </p>
    );
  };

  // Bootstrap advances itself (it owns its "Create admin" submit); every later step uses
  // the shell's generic Continue, gated on that step's server-derived completion.
  const advances = currentId !== "bootstrap" && index < WIZARD_STEPS.length - 1;

  return (
    <WizardShell
      steps={WIZARD_STEPS}
      currentId={currentId}
      statusById={statusById}
      title={copy?.title ?? step?.title ?? "Setup"}
      description={copy?.description}
      onBack={index > 0 ? () => setVisited(WIZARD_STEPS[index - 1]?.id) : undefined}
      onNext={advances ? () => setVisited(WIZARD_STEPS[index + 1]?.id) : undefined}
      nextDisabled={advances && !isStepDone(currentId, { checks, isAuthenticated })}
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
