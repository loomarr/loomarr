import type { DatabaseCheck } from "@loomarr/api";
import { systemApi, unwrap } from "@loomarr/api";
import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { useCallback, useEffect, useState } from "react";
import type { MigrationStep } from "@/components/loomarr";
import { DatabaseMigration, ErrorState } from "@/components/loomarr";
import { useLoomarrEventListener } from "@/events";

// Settings → System → Database (§18, V11) — the SQLite → PostgreSQL migration stepper.
//
// The stage lives HERE rather than in the component so a background refetch cannot reset
// the operator's position mid-migration. The server owns the facts (what backend is live,
// what has been copied, whether parity matched); the page owns only where the operator is
// standing, which is genuinely client state.
const DatabasePage = () => {
  const queryClient = useQueryClient();
  const [step, setStep] = useState<MigrationStep | null>(null);
  const [dsn, setDsn] = useState("");
  const [checks, setChecks] = useState<DatabaseCheck[]>([]);
  const [passed, setPassed] = useState(false);
  // After migrate acknowledges, a dropped connection is expected rather than an error
  // screen. Poll until the fresh generation answers on PostgreSQL or carries a failure.
  const status = systemApi.useSystemDatabaseStatus({
    query: { refetchInterval: step === "reconnect" ? 1_000 : false },
  });

  // A `database` frame means the migration moved. Refetch rather than patching from the
  // payload: the frame is a latency optimization and GET is the source of truth (§8), and
  // for a data migration specifically a UI that invented progress it had not been told
  // about would be actively misleading.
  const refetchStatus = useCallback(() => {
    void queryClient.invalidateQueries({
      queryKey: systemApi.getSystemDatabaseStatusQueryKey(),
    });
  }, [queryClient]);
  useLoomarrEventListener({ onDatabase: refetchStatus });

  const preflight = systemApi.useSystemDatabasePreflight({
    mutation: {
      onSuccess: (res) => {
        if (res.status !== 200) return;
        setChecks(res.data.checks ?? []);
        setPassed(res.data.passed ?? false);
        setStep("preflight");
      },
    },
  });

  const backup = systemApi.useSystemDatabaseBackup({
    mutation: { onSuccess: refetchStatus },
  });

  const migrate = systemApi.useSystemDatabaseMigrate({
    mutation: {
      onSuccess: () => {
        refetchStatus();
        // The request only queues the process-level operation. Losing the connection
        // after this point is success-shaped; wait for the fresh generation.
        setStep("reconnect");
      },
      onError: refetchStatus,
    },
  });

  const view = unwrap(status.data);
  useEffect(() => {
    if (step !== "reconnect" || view?.phase !== "failed") return;
    // The failed generation deliberately forgets its preflight/backup authorization.
    // Return to the beginning so a retry proves the target is empty again.
    setStep(null);
    setChecks([]);
    setPassed(false);
  }, [step, view?.phase]);

  if (status.error && step !== "reconnect") {
    return <ErrorState error={status.error} onRetry={() => status.refetch()} />;
  }
  if (view == null) return null;

  // No schema change is needed to surface an env pin: Migrate rejects it before queueing,
  // and the existing problem detail gives the page the permanent explanation state.
  const envPinned = migrate.error?.detail?.includes("pinned") ?? false;

  const pending =
    (preflight.isPending && "preflight") ||
    (backup.isPending && "backup") ||
    (migrate.isPending && "migrate") ||
    null;

  const error =
    view.error ?? migrate.error?.detail ?? backup.error?.detail ?? preflight.error?.detail ?? null;

  return (
    <div className="h-full overflow-auto p-6">
      <div className="mb-4">
        <h1 className="font-semibold text-xl">Database</h1>
        <p className="mt-1 text-muted-foreground text-sm">
          Which database Loomarr stores everything in, and how to move to another one.
        </p>
      </div>
      <DatabaseMigration
        status={view}
        step={step}
        onStepChange={setStep}
        dsn={dsn}
        onDsnChange={setDsn}
        checks={checks}
        preflightPassed={passed}
        onPreflight={() => preflight.mutate({ data: { dsn } })}
        onBackup={() => backup.mutate()}
        onMigrate={() => migrate.mutate({ data: { dsn } })}
        pending={pending}
        error={error}
        envPinned={envPinned}
      />
    </div>
  );
};

const Route = createFileRoute("/_authed/settings/system/database")({
  component: DatabasePage,
});

export { Route };
