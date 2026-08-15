import { formatBytes } from "@loomarr/core";
import { AlertTriangle, Check, Database, Lock } from "lucide-react";
import { Badge, Button, Input, Label } from "@/components/ui";
import { cn } from "@/lib";
import type { DatabaseMigrationProps, MigrationStep } from "./database-migration.type";

// DatabaseMigration — the SQLite → PostgreSQL stepper (§18, V11; v2 mock System → Database).
//
// Four browser stages: connect → preflight → backup → reconnect. Copy, independent
// verification, bootstrap commit and restart are one process-owned operation after the
// current generation drains; the browser must not offer separate commit controls.
//
// ⚠ **The gate this renders is not the gate.** The Migrate button is disabled until a
// backup exists, but that is a hint; the server refuses a migrate call without one
// regardless of what the client believes (see internal/app/database.go). Disabling here is
// courtesy — it stops someone clicking a button that cannot work — and the copy says
// "required, not suggested" because that is literally true, not a scare tactic.

const STEPS: { key: MigrationStep; label: string }[] = [
  { key: "connect", label: "Connect" },
  { key: "preflight", label: "Preflight" },
  { key: "backup", label: "Backup" },
  { key: "reconnect", label: "Reconnect" },
];

const DatabaseMigration = ({
  status,
  step,
  onStepChange,
  dsn,
  onDsnChange,
  checks,
  preflightPassed,
  onPreflight,
  onBackup,
  onMigrate,
  pending,
  error,
  envPinned,
  className,
}: DatabaseMigrationProps) => {
  const at = step === null ? -1 : STEPS.findIndex((s) => s.key === step);
  const backup = status.backup;

  // Already on Postgres: say so rather than rendering nothing. An absent stepper reads as
  // a missing feature; this reads as an answered question.
  if (!status.canMigrate) {
    return (
      <section className={cn("rounded-lg border border-border p-4", className)}>
        <div className="flex items-center gap-2">
          <Database className="size-4 text-muted-foreground" aria-hidden />
          <h3 className="font-medium text-sm">Running on PostgreSQL</h3>
        </div>
        <p className="mt-2 text-muted-foreground text-sm">
          Nothing to migrate. Back PostgreSQL up with your usual tooling. Loomarr does not try to own that,
          because an operator already running Postgres has a strategy better than anything it would invent.
        </p>
      </section>
    );
  }

  return (
    <section className={cn("rounded-lg border border-border", className)}>
      <header className="border-border border-b p-4">
        <div className="flex flex-wrap items-center gap-2">
          <Database className="size-4 text-tune" aria-hidden />
          <h3 className="font-medium text-sm">Move to PostgreSQL</h3>
          <Badge className="font-mono text-2xs uppercase">{status.backend}</Badge>
        </div>
        <p className="mt-1.5 text-muted-foreground text-sm">
          {step === null
            ? "Four steps. Your SQLite file is never deleted."
            : `Step ${at + 1} of 4 · the switch happens only after an independent parity check.`}
        </p>
        {/* Why anyone would do this — and why they might not need to. Stating the SQLite
            constraint is the honest framing: Postgres is not "better", it buys replicas. */}
        <p className="mt-2 text-muted-foreground text-xs">
          SQLite: run exactly one instance: a second process writing the same file will corrupt it. PostgreSQL
          enables replicas, and is worth it if you already run Postgres and would rather back up one thing.
        </p>
      </header>

      {envPinned ? (
        // An env pin wins at boot, so an atomic copy-and-switch cannot commit.
        <div className="flex gap-3 p-4">
          <Lock className="mt-0.5 size-4 shrink-0 text-lock" aria-hidden />
          <div>
            <p className="text-sm">DATABASE_URL is pinned by the environment.</p>
            <p className="mt-1 text-muted-foreground text-sm">
              In-app migration is unavailable because an environment variable always wins at boot. Change
              DATABASE_URL where Loomarr is launched and restart; Loomarr will not make a copy that can
              silently diverge from the database it keeps using.
            </p>
          </div>
        </div>
      ) : (
        <>
          <ol className="flex flex-wrap items-center gap-x-2 gap-y-2 border-border border-b px-4 py-3">
            {STEPS.map((s, i) => {
              const done = at > i;
              const current = at === i;
              return (
                <li key={s.key} className="flex items-center gap-2">
                  <span
                    className={cn(
                      "flex size-5 items-center justify-center rounded-full border font-mono text-2xs",
                      done && "border-signal bg-signal-tint-15 text-signal",
                      current && "border-tune bg-tune-tint-15 text-tune",
                      !done && !current && "border-border text-muted-foreground",
                    )}
                  >
                    {done ? <Check className="size-3" aria-hidden /> : i + 1}
                  </span>
                  <span
                    className={cn(
                      "text-xs",
                      current ? "font-medium text-foreground" : "text-muted-foreground",
                    )}
                  >
                    {s.label}
                  </span>
                  {i < STEPS.length - 1 && (
                    <span className="text-muted-foreground text-xs" aria-hidden>
                      ›
                    </span>
                  )}
                </li>
              );
            })}
          </ol>

          <div className="p-4">
            {step === null && <Button onClick={() => onStepChange("connect")}>Move to PostgreSQL</Button>}

            {step === "connect" && (
              <div className="flex max-w-xl flex-col gap-3">
                <div className="flex flex-col gap-1.5">
                  <Label htmlFor="migration-dsn">PostgreSQL connection string</Label>
                  <Input
                    id="migration-dsn"
                    value={dsn}
                    onChange={(e) => onDsnChange(e.target.value)}
                    placeholder="postgres://loomarr:password@postgres:5432/loomarr"
                    aria-describedby="migration-dsn-doc"
                  />
                  <p id="migration-dsn-doc" className="text-muted-foreground text-xs">
                    sslmode=prefer negotiates TLS and falls back, which is right for a docker network. Use
                    require for a database across the internet.
                  </p>
                </div>
                <Button
                  onClick={onPreflight}
                  disabled={dsn.trim() === "" || pending === "preflight"}
                  className="self-start"
                >
                  {pending === "preflight" ? "Running preflight…" : "Run preflight"}
                </Button>
              </div>
            )}

            {step === "preflight" && (
              <div className="flex flex-col gap-3">
                <ul className="flex flex-col gap-2">
                  {checks.map((c) => (
                    <li key={c.name} className="flex items-start gap-2">
                      <span
                        className={cn(
                          "mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-full font-mono text-2xs",
                          c.ok ? "bg-signal-tint-15 text-signal" : "bg-onair-tint-15 text-onair-300",
                        )}
                        aria-hidden
                      >
                        {c.ok ? "✓" : "✕"}
                      </span>
                      <span className="text-sm">
                        {c.name}
                        <span className={cn("ml-2", c.ok ? "text-muted-foreground" : "text-onair-300")}>
                          {c.detail}
                        </span>
                      </span>
                    </li>
                  ))}
                </ul>
                <div className="flex gap-2">
                  <Button onClick={() => onStepChange("backup")} disabled={!preflightPassed}>
                    Continue
                  </Button>
                  <Button variant="ghost" onClick={() => onStepChange("connect")}>
                    Back
                  </Button>
                </div>
                {!preflightPassed && checks.length > 0 && (
                  <p className="text-muted-foreground text-xs">
                    Every check has to pass. Fix the target and run preflight again.
                  </p>
                )}
              </div>
            )}

            {step === "backup" && (
              <div className="flex flex-col gap-3">
                <p className="text-sm">
                  A backup is required, not suggested. It&rsquo;s the only thing that makes this reversible.
                  Loomarr writes it before touching either database.
                </p>
                <p className="font-mono text-muted-foreground text-xs">
                  {backup
                    ? `${backup.path} · ${formatBytes(backup.bytes)} · written just now`
                    : "no backup yet for this migration"}
                </p>
                <div className="flex gap-2">
                  <Button onClick={onBackup} disabled={pending === "backup"}>
                    {pending === "backup" ? "Writing backup…" : "Back up now"}
                  </Button>
                  {/* Disabled until the backup exists. The SERVER refuses regardless — this
                      only stops a click that could not have worked. */}
                  <Button onClick={onMigrate} disabled={!backup || pending === "migrate"}>
                    {pending === "migrate" ? "Requesting restart…" : "Migrate and restart"}
                  </Button>
                </div>
                <p className="text-muted-foreground text-xs">
                  Loomarr will acknowledge the request, drain connections, close SQLite, copy and verify the
                  data, switch its boot configuration, and restart. This page reconnects automatically.
                </p>
              </div>
            )}

            {step === "reconnect" && (
              <div className="flex flex-col gap-3">
                <p className="font-mono text-tune text-xs">migration accepted · waiting for Loomarr</p>
                <p className="text-sm">
                  The connection may disappear while Loomarr drains, copies, verifies, switches databases, and
                  restarts. Keep this page open; it will reconnect automatically.
                </p>
                <p className="text-muted-foreground text-xs">
                  If any step fails, Loomarr comes back on SQLite and explains why. The SQLite file is never
                  deleted.
                </p>
              </div>
            )}

            {/* The failure copy is deliberately specific about what was NOT lost: the
                operator's first question after a failed migration is whether their data
                survived, and a generic "migration failed" leaves them to guess. */}
            {error && (
              <p className="mt-3 flex items-start gap-2 text-onair-300 text-sm">
                <AlertTriangle className="mt-0.5 size-4 shrink-0" aria-hidden />
                {error}
              </p>
            )}
          </div>
        </>
      )}
    </section>
  );
};

export { DatabaseMigration };
