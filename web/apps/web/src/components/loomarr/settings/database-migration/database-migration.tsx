import { formatBytes } from "@loomarr/core";
import { AlertTriangle, Check, Database, Lock } from "lucide-react";
import { Badge, Button, Caption, Input, Label } from "@/components/ui";
import { cn } from "@/lib";
import type { DatabaseMigrationProps, MigrationStep } from "./database-migration.type";

// DatabaseMigration — the SQLite → PostgreSQL stepper (§18, V11; v2 mock System → Database).
//
// Six stages: connect → preflight → backup → migrate → verify → restart. They are separate
// stages rather than one button because each is a decision point — preflight can send you
// back to fix the target, and the backup between them is a gate rather than a step.
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
  { key: "migrate", label: "Migrate" },
  { key: "verify", label: "Verify" },
  { key: "restart", label: "Restart" },
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
  onSwitchover,
  pending,
  error,
  envPinned,
  className,
}: DatabaseMigrationProps) => {
  const at = step === null ? -1 : STEPS.findIndex((s) => s.key === step);
  const backup = status.backup;
  const tables = status.tables ?? [];

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
            ? "Six steps, reversible until the switch-over. Your SQLite file is never deleted."
            : `Step ${at + 1} of 6 · nothing is switched over until verify passes.`}
        </p>
        {/* Why anyone would do this — and why they might not need to. Stating the SQLite
            constraint is the honest framing: Postgres is not "better", it buys replicas. */}
        <p className="mt-2 text-muted-foreground text-xs">
          SQLite: run exactly one instance: a second process writing the same file will corrupt it. PostgreSQL
          enables replicas, and is worth it if you already run Postgres and would rather back up one thing.
        </p>
      </header>

      {envPinned ? (
        // An env pin wins at boot, so Loomarr can copy the data but cannot record the
        // switch. Offering the full stepper would promise a switchover the next boot
        // would silently undo — the server refuses it for the same reason.
        <div className="flex gap-3 p-4">
          <Lock className="mt-0.5 size-4 shrink-0 text-lock" aria-hidden />
          <div>
            <p className="text-sm">DATABASE_URL is pinned by the environment.</p>
            <p className="mt-1 text-muted-foreground text-sm">
              Loomarr can copy your data to PostgreSQL, but it cannot record the switch. An environment
              variable always wins at boot. Migrate the data, then change DATABASE_URL where you set it and
              restart.
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
                    {pending === "migrate" ? "Migrating…" : "Migrate data"}
                  </Button>
                </div>
              </div>
            )}

            {(step === "migrate" || step === "verify") && (
              <div className="flex flex-col gap-3">
                <p className="font-mono text-tune text-xs">
                  {status.phase === "failed"
                    ? "aborted: source database untouched"
                    : "copying table by table · source stays read-only"}
                </p>
                <ul className="flex flex-col gap-2">
                  {tables.map((t) => {
                    const pct = t.source === 0 ? 100 : Math.round((t.copied / t.source) * 100);
                    return (
                      <li key={t.table} className="flex items-center gap-3">
                        <span className="w-32 shrink-0 truncate font-mono text-xs">{t.table}</span>
                        <span
                          className="h-1.5 flex-1 overflow-hidden rounded-full bg-static-800"
                          role="progressbar"
                          aria-valuenow={pct}
                          aria-valuemin={0}
                          aria-valuemax={100}
                          aria-label={`${t.table} copy progress`}
                        >
                          <span
                            className={cn(
                              "block h-full rounded-full",
                              status.phase === "failed" ? "bg-onair" : pct === 100 ? "bg-signal" : "bg-tune",
                            )}
                            style={{ width: `${pct}%` }}
                          />
                        </span>
                        <Caption className="w-24 shrink-0 text-right">{`${t.copied}/${t.source}`}</Caption>
                      </li>
                    );
                  })}
                </ul>

                {status.parity === "match" && (
                  <div className="flex flex-col gap-3">
                    <p className="text-sm">
                      Row-count parity, table by table. Nothing switches over until every row is accounted
                      for.
                      <span className="ml-2 font-mono text-2xs text-signal uppercase">match</span>
                    </p>
                    <Button onClick={onSwitchover} disabled={pending === "switchover"}>
                      {pending === "switchover" ? "Switching over…" : "Switch over"}
                    </Button>
                  </div>
                )}
                {status.parity === "mismatch" && (
                  <p className="flex items-start gap-2 text-onair-300 text-sm">
                    <AlertTriangle className="mt-0.5 size-4 shrink-0" aria-hidden />
                    Row counts do not match, so nothing was switched over. Your SQLite database was only read
                    from, this install is still running on it.
                  </p>
                )}
              </div>
            )}

            {step === "restart" && (
              <div className="flex flex-col gap-2">
                <p className="text-sm">
                  Loomarr will use the migrated database on its next start. With a restart policy it will be
                  back on PostgreSQL in a few seconds; without one, start it manually.
                </p>
                <p className="text-muted-foreground text-xs">
                  Your SQLite file is left in place, untouched, as a fallback.
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
