import type { DatabaseStatus } from "@loomarr/api";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { DatabaseMigration } from "./database-migration";
import type { DatabaseMigrationProps } from "./database-migration.type";

const status = (over: Partial<DatabaseStatus> = {}): DatabaseStatus => ({
  backend: "sqlite",
  canMigrate: true,
  phase: "idle",
  tables: [],
  parity: "unknown",
  ...over,
});

const Harness = (over: Partial<DatabaseMigrationProps> = {}) => (
  <DatabaseMigration
    status={status()}
    step={null}
    onStepChange={vi.fn()}
    dsn=""
    onDsnChange={vi.fn()}
    checks={[]}
    preflightPassed={false}
    onPreflight={vi.fn()}
    onBackup={vi.fn()}
    onMigrate={vi.fn()}
    onSwitchover={vi.fn()}
    {...over}
  />
);

describe("DatabaseMigration", () => {
  // ⚠ THE PHASE GATE, as far as the UI can express it. The server refuses regardless
  // (internal/app/database.go), so this is not the enforcement — but a Migrate button that
  // looks clickable and then 409s is a worse experience than one that explains itself.
  it("disables Migrate until a backup exists", () => {
    const { rerender } = render(<Harness step="backup" />);
    expect(screen.getByRole("button", { name: "Migrate data" })).toBeDisabled();

    rerender(
      <Harness
        step="backup"
        status={status({ backup: { path: "/data/backups/x.db", bytes: 4096, writtenAt: 1 } })}
      />,
    );
    expect(screen.getByRole("button", { name: "Migrate data" })).toBeEnabled();
  });

  // The copy is load-bearing: an operator who reads "recommended" skips it, and the whole
  // reversibility story rests on the file existing.
  it("says the backup is required, not suggested", () => {
    render(<Harness step="backup" />);
    expect(screen.getByText(/required, not suggested/i)).toBeInTheDocument();
  });

  it("blocks Continue until every preflight check passes", () => {
    const checks = [
      { name: "Reachable", detail: "connected in 3ms", ok: true },
      { name: "Target is empty", detail: "10 table(s) already present", ok: false },
    ];
    const { rerender } = render(<Harness step="preflight" checks={checks} preflightPassed={false} />);
    expect(screen.getByRole("button", { name: "Continue" })).toBeDisabled();
    // The FAILING check's detail has to be visible — "preflight failed" alone sends the
    // operator to debug the wrong thing.
    expect(screen.getByText(/10 table\(s\) already present/)).toBeInTheDocument();

    rerender(<Harness step="preflight" checks={checks.map((c) => ({ ...c, ok: true }))} preflightPassed />);
    expect(screen.getByRole("button", { name: "Continue" })).toBeEnabled();
  });

  it("offers switchover only once parity matches", () => {
    const tables = [{ table: "titles", source: 1204, copied: 1204 }];
    const { rerender } = render(
      <Harness step="verify" status={status({ phase: "migrating", tables, parity: "unknown" })} />,
    );
    expect(screen.queryByRole("button", { name: "Switch over" })).not.toBeInTheDocument();

    rerender(<Harness step="verify" status={status({ phase: "verified", tables, parity: "match" })} />);
    expect(screen.getByRole("button", { name: "Switch over" })).toBeEnabled();
  });

  // A mismatch must state what was NOT lost. The operator's first question after a failed
  // migration is whether their data survived.
  it("says the source is untouched when parity fails", () => {
    render(
      <Harness
        step="verify"
        status={status({
          phase: "failed",
          parity: "mismatch",
          tables: [{ table: "titles", source: 1204, copied: 506 }],
        })}
      />,
    );
    expect(screen.getByText(/only read from/i)).toBeInTheDocument();
    expect(screen.getByText(/still running on it/i)).toBeInTheDocument();
  });

  it("reports per-table progress as accessible progressbars", () => {
    render(
      <Harness
        step="migrate"
        status={status({
          phase: "migrating",
          tables: [
            { table: "titles", source: 1000, copied: 500 },
            { table: "users", source: 4, copied: 4 },
          ],
        })}
      />,
    );
    const bars = screen.getAllByRole("progressbar");
    expect(bars).toHaveLength(2);
    expect(bars[0]).toHaveAttribute("aria-valuenow", "50");
    expect(bars[1]).toHaveAttribute("aria-valuenow", "100");
  });

  // An install already on Postgres gets an answer, not an absence. A missing stepper reads
  // as a broken feature.
  it("explains itself on a Postgres install instead of rendering nothing", () => {
    render(<Harness status={status({ backend: "postgres", canMigrate: false })} />);
    expect(screen.getByText(/Running on PostgreSQL/i)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Move to PostgreSQL/i })).not.toBeInTheDocument();
  });

  // An env pin wins at boot, so offering the switchover would promise something the next
  // boot silently undoes.
  it("explains an env-pinned DATABASE_URL rather than offering the stepper", () => {
    render(<Harness envPinned />);
    expect(screen.getByText(/pinned by the environment/i)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Move to PostgreSQL" })).not.toBeInTheDocument();
  });

  it("runs preflight with the entered connection string", async () => {
    const onPreflight = vi.fn();
    render(<Harness step="connect" dsn="postgres://u:p@db:5432/loomarr" onPreflight={onPreflight} />);
    await userEvent.click(screen.getByRole("button", { name: "Run preflight" }));
    expect(onPreflight).toHaveBeenCalled();
  });

  it("cannot run preflight without a connection string", () => {
    render(<Harness step="connect" dsn="" />);
    expect(screen.getByRole("button", { name: "Run preflight" })).toBeDisabled();
  });

  it("marks completed stages in the stepper", () => {
    render(<Harness step="backup" />);
    const steps = screen.getByRole("list");
    // Connect and Preflight are behind us; Backup is current.
    expect(within(steps).getByText("Connect")).toBeInTheDocument();
    expect(within(steps).getByText("Backup")).toBeInTheDocument();
  });
});
