import type { DatabaseCheck } from "@loomarr/api";
import { systemApi, unwrap } from "@loomarr/api";
import { useQueryClient } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { useCallback, useState } from "react";
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
  const status = systemApi.useSystemDatabaseStatus();

  const [step, setStep] = useState<MigrationStep | null>(null);
  const [dsn, setDsn] = useState("");
  const [checks, setChecks] = useState<DatabaseCheck[]>([]);
  const [passed, setPassed] = useState(false);

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
        // Straight to verify: the call returns only once the copy AND the parity check
        // have finished, so there is no window where "migrating" is still true.
        setStep("verify");
      },
      onError: refetchStatus,
    },
  });

  const switchover = systemApi.useSystemDatabaseSwitchover({
    mutation: { onSuccess: () => setStep("restart") },
  });

  if (status.error) return <ErrorState error={status.error} onRetry={() => status.refetch()} />;
  const view = unwrap(status.data);
  if (view == null) return null;

  // A pinned DATABASE_URL is reported by the switchover refusing, but the stepper should
  // say so BEFORE the operator has taken a backup and copied every row — so the page
  // treats a switchover failure mentioning the pin as the env-pinned state.
  const envPinned = switchover.error?.detail?.includes("pinned") ?? false;

  const pending =
    (preflight.isPending && "preflight") ||
    (backup.isPending && "backup") ||
    (migrate.isPending && "migrate") ||
    (switchover.isPending && "switchover") ||
    null;

  const error =
    migrate.error?.detail ??
    backup.error?.detail ??
    switchover.error?.detail ??
    preflight.error?.detail ??
    null;

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
        onSwitchover={() => switchover.mutate({ data: { dsn } })}
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
