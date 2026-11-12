import { formatBytes } from "@loomarr/core/format";
import { Action, Field, FieldHint, Icon, icons, Stack, StatusLabel } from "@loomarr/design-system";
import { cn } from "@/lib/utils";
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
      <section className={cn("border-border rounded-lg border p-4", className)}>
        <Stack align="center" direction="row" gap="inline">
          <Icon decorative glyph={icons.database} size="compact" tone="muted" />
          <h3 className="text-sm font-medium">Running on PostgreSQL</h3>
        </Stack>
        <p className="text-muted-foreground mt-2 text-sm">
          Nothing to migrate. Back PostgreSQL up with your usual tooling. Loomarr does not try to own that,
          because an operator already running Postgres has a strategy better than anything it would invent.
        </p>
      </section>
    );
  }

  return (
    <section className={cn("border-border rounded-lg border", className)}>
      <header className="border-border border-b p-4">
        <Stack align="center" direction="row" gap="inline" wrap>
          <Icon decorative glyph={icons.database} size="compact" tone="info" />
          <h3 className="text-sm font-medium">Move to PostgreSQL</h3>
          <StatusLabel tone="ok">{status.backend}</StatusLabel>
        </Stack>
        <p className="text-muted-foreground mt-1.5 text-sm">
          {step === null
            ? "Four steps. Your SQLite file is never deleted."
            : `Step ${at + 1} of 4 · the switch happens only after an independent parity check.`}
        </p>
        {/* Why anyone would do this — and why they might not need to. Stating the SQLite
            constraint is the honest framing: Postgres is not "better", it buys replicas. */}
        <p className="text-muted-foreground mt-2 text-xs">
          SQLite: run exactly one instance: a second process writing the same file will corrupt it. PostgreSQL
          enables replicas, and is worth it if you already run Postgres and would rather back up one thing.
        </p>
      </header>

      {envPinned ? (
        // An env pin wins at boot, so an atomic copy-and-switch cannot commit.
        <Stack className="p-4" direction="row" gap="control">
          <Stack className="mt-0.5" direction="row" render="span">
            <Icon decorative glyph={icons.locked} size="compact" tone="success" />
          </Stack>
          <div>
            <p className="text-sm">DATABASE_URL is pinned by the environment.</p>
            <p className="text-muted-foreground mt-1 text-sm">
              In-app migration is unavailable because an environment variable always wins at boot. Change
              DATABASE_URL where Loomarr is launched and restart; Loomarr will not make a copy that can
              silently diverge from the database it keeps using.
            </p>
          </div>
        </Stack>
      ) : (
        <>
          <Stack
            align="center"
            className="border-border border-b px-4 py-3"
            direction="row"
            gap="inline"
            render="ol"
            wrap
          >
            {STEPS.map((s, i) => {
              const done = at > i;
              const current = at === i;
              return (
                <Stack align="center" direction="row" gap="inline" key={s.key} render="li">
                  <span
                    className={cn(
                      "text-2xs flex size-5 items-center justify-center rounded-full border font-mono",
                      done && "border-signal bg-signal-tint-15 text-signal",
                      current && "border-tune bg-tune-tint-15 text-tune",
                      !done && !current && "border-border text-muted-foreground",
                    )}
                  >
                    {done ? <Icon decorative glyph={icons.selected} size="compact" tone="inherit" /> : i + 1}
                  </span>
                  <span
                    className={cn(
                      "text-xs",
                      current ? "text-foreground font-medium" : "text-muted-foreground",
                    )}
                  >
                    {s.label}
                  </span>
                  {i < STEPS.length - 1 && (
                    <span className="text-muted-foreground text-xs" aria-hidden>
                      ›
                    </span>
                  )}
                </Stack>
              );
            })}
          </Stack>

          <div className="p-4">
            {step === null && (
              <Stack direction="row">
                <Action onPress={() => onStepChange("connect")}>Move to PostgreSQL</Action>
              </Stack>
            )}

            {step === "connect" && (
              <Stack className="max-w-xl" gap="control">
                <Stack gap="1.5">
                  <Field
                    aria-describedby="migration-dsn-doc"
                    autoCapitalize="none"
                    autoComplete="off"
                    autoCorrect={false}
                    id="migration-dsn"
                    keyboardType="url"
                    label="PostgreSQL connection string"
                    onChangeText={onDsnChange}
                    placeholder="postgres://loomarr:password@postgres:5432/loomarr"
                    value={dsn}
                  />
                  <FieldHint id="migration-dsn-doc">
                    sslmode=prefer negotiates TLS and falls back, which is right for a docker network. Use
                    require for a database across the internet.
                  </FieldHint>
                </Stack>
                <Action
                  disabled={dsn.trim() === "" || pending === "preflight"}
                  onPress={onPreflight}
                  style={{ alignSelf: "flex-start" }}
                >
                  {pending === "preflight" ? "Running preflight…" : "Run preflight"}
                </Action>
              </Stack>
            )}

            {step === "preflight" && (
              <Stack gap="control">
                <Stack gap="inline" render="ul">
                  {checks.map((c) => (
                    <Stack align="start" direction="row" gap="inline" key={c.name} render="li">
                      <span
                        className={cn(
                          "text-2xs mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-full font-mono",
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
                    </Stack>
                  ))}
                </Stack>
                <Stack direction="row" gap="inline">
                  <Action disabled={!preflightPassed} onPress={() => onStepChange("backup")}>
                    Continue
                  </Action>
                  <Action onPress={() => onStepChange("connect")} variant="ghost">
                    Back
                  </Action>
                </Stack>
                {!preflightPassed && checks.length > 0 && (
                  <FieldHint>Every check has to pass. Fix the target and run preflight again.</FieldHint>
                )}
              </Stack>
            )}

            {step === "backup" && (
              <Stack gap="control">
                <p className="text-sm">
                  A backup is required, not suggested. It&rsquo;s the only thing that makes this reversible.
                  Loomarr writes it before touching either database.
                </p>
                <p className="text-muted-foreground font-mono text-xs">
                  {backup
                    ? `${backup.path} · ${formatBytes(backup.bytes)} · written just now`
                    : "no backup yet for this migration"}
                </p>
                <Stack direction="row" gap="inline">
                  <Action disabled={pending === "backup"} onPress={onBackup}>
                    {pending === "backup" ? "Writing backup…" : "Back up now"}
                  </Action>
                  {/* Disabled until the backup exists. The SERVER refuses regardless — this
                      only stops a click that could not have worked. */}
                  <Action disabled={!backup || pending === "migrate"} onPress={onMigrate}>
                    {pending === "migrate" ? "Requesting restart…" : "Migrate and restart"}
                  </Action>
                </Stack>
                <p className="text-muted-foreground text-xs">
                  Loomarr will acknowledge the request, drain connections, close SQLite, copy and verify the
                  data, switch its boot configuration, and restart. This page reconnects automatically.
                </p>
              </Stack>
            )}

            {step === "reconnect" && (
              <Stack gap="control">
                <p className="text-tune font-mono text-xs">migration accepted · waiting for Loomarr</p>
                <p className="text-sm">
                  The connection may disappear while Loomarr drains, copies, verifies, switches databases, and
                  restarts. Keep this page open; it will reconnect automatically.
                </p>
                <p className="text-muted-foreground text-xs">
                  If any step fails, Loomarr comes back on SQLite and explains why. The SQLite file is never
                  deleted.
                </p>
              </Stack>
            )}

            {/* The failure copy is deliberately specific about what was NOT lost: the
                operator's first question after a failed migration is whether their data
                survived, and a generic "migration failed" leaves them to guess. */}
            {error && (
              <Stack
                align="start"
                className="text-onair-300 mt-3 text-sm"
                direction="row"
                gap="inline"
                render="p"
              >
                <Stack className="mt-0.5" direction="row" render="span">
                  <Icon decorative glyph={icons.alert} size="compact" tone="inherit" />
                </Stack>
                {error}
              </Stack>
            )}
          </div>
        </>
      )}
    </section>
  );
};

export { DatabaseMigration };
