import type { BackupList } from "@loomarr/api";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { BackupPanel } from "./backup-panel";

const base: BackupList = {
  supported: true,
  dir: "/data/backups",
  schedule: "0 30 3 * * *",
  retain: 7,
  backups: [
    { name: "loomarr-2026-07-29-033000.db", bytes: 4_404_019, writtenAt: 1_800_000_000 },
    { name: "loomarr-2026-07-28-033000.db", bytes: 4_298_342, writtenAt: 1_799_913_600 },
  ],
};

const noop = () => {};

describe("BackupPanel", () => {
  it("lists each backup with its size and a download action", () => {
    render(<BackupPanel list={base} onBackUpNow={noop} onDownload={noop} />);

    expect(screen.getByText("loomarr-2026-07-29-033000.db")).toBeInTheDocument();
    expect(screen.getByText("loomarr-2026-07-28-033000.db")).toBeInTheDocument();
    expect(screen.getByText("4.2 MB")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Download loomarr-2026-07-29-033000.db" })).toBeInTheDocument();
  });

  // ⚠ Download must carry the FILENAME, not a row index. The server resolves the name
  // against its own listing, and an index would silently address a different backup the
  // moment retention prunes one between render and click.
  it("downloads by filename", async () => {
    const onDownload = vi.fn();
    render(<BackupPanel list={base} onBackUpNow={noop} onDownload={onDownload} />);

    await userEvent.click(screen.getByRole("button", { name: "Download loomarr-2026-07-28-033000.db" }));

    expect(onDownload).toHaveBeenCalledWith("loomarr-2026-07-28-033000.db");
  });

  it("writes a backup on demand", async () => {
    const onBackUpNow = vi.fn();
    render(<BackupPanel list={base} onBackUpNow={onBackUpNow} onDownload={noop} />);

    await userEvent.click(screen.getByRole("button", { name: /back up now/i }));

    expect(onBackUpNow).toHaveBeenCalledOnce();
  });

  it("disables the button while a backup is in flight", () => {
    render(<BackupPanel list={base} onBackUpNow={noop} onDownload={noop} pending />);
    expect(screen.getByRole("button", { name: /backing up/i })).toBeDisabled();
  });

  it("surfaces a write failure", () => {
    render(
      <BackupPanel
        list={base}
        onBackUpNow={noop}
        onDownload={noop}
        error="The backup could not be written."
      />,
    );
    expect(screen.getByRole("alert")).toHaveTextContent("The backup could not be written.");
  });

  // A fresh install has none. The empty state has to point somewhere, or it reads as the
  // feature having failed to load.
  it("explains an empty directory rather than rendering nothing", () => {
    render(<BackupPanel list={{ ...base, backups: [] }} onBackUpNow={noop} onDownload={noop} />);
    expect(screen.getByText(/no backups yet/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /back up now/i })).toBeInTheDocument();
  });

  it("states the schedule and retention in force", () => {
    render(<BackupPanel list={base} onBackUpNow={noop} onDownload={noop} />);
    expect(screen.getByText(/keeps 7 backups/)).toBeInTheDocument();
    expect(screen.getByText(/0 30 3 \* \* \*/)).toBeInTheDocument();
  });

  // ⚠ retain=0 means keep everything. "keeps 0 backups" would describe the opposite of
  // what the server does.
  it("reads retain=0 as keeping everything", () => {
    render(<BackupPanel list={{ ...base, retain: 0 }} onBackUpNow={noop} onDownload={noop} />);
    expect(screen.getByText(/keeps every backup/)).toBeInTheDocument();
    expect(screen.queryByText(/keeps 0/)).not.toBeInTheDocument();
  });

  // ⚠ Postgres gets an EXPLANATION, not an empty table. An empty list reads as breakage
  // on the one install where the operator is correctly using pg_dump.
  it("explains that Postgres is backed up with pg_dump, offering no controls", () => {
    render(
      <BackupPanel list={{ ...base, supported: false, backups: [] }} onBackUpNow={noop} onDownload={noop} />,
    );
    expect(screen.getByText(/pg_dump/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /back up now/i })).not.toBeInTheDocument();
  });

  // There is no in-app Restore, and the panel says so — otherwise its absence reads as an
  // oversight rather than a decision.
  it("says restore is a command-line operation", () => {
    render(<BackupPanel list={base} onBackUpNow={noop} onDownload={noop} />);
    expect(screen.getByText(/restore is a command-line operation/i)).toBeInTheDocument();
  });
});
