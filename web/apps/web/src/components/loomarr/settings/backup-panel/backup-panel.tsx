import { formatBytes, formatRelative, pluralize } from "@loomarr/core";
import { AlertTriangle, Database, Download, HardDriveDownload } from "lucide-react";
import { Button } from "@/components/ui";
import { cn } from "@/lib";
import type { BackupPanelProps } from "./backup-panel.type";

// BackupPanel — Settings → System → Backup (§16, V12; v2 mock System → Backup).
//
// A backup is the whole instance: settings, channels, people, and the generated secrets.
// That sentence is the panel's opening line rather than a footnote, because the file this
// page hands someone is a credential and the page is where they learn it.
//
// ⚠ **There is no Restore button, deliberately.** Restoring replaces the database the app
// is running on — including the accounts and sessions that authorize the click. It is a
// CLI operation (stop, replace the file, start), and the panel says so rather than leaving
// its absence to read as an oversight.

const BackupPanel = ({ list, onBackUpNow, onDownload, pending, error, className }: BackupPanelProps) => {
  const backups = list.backups ?? [];

  // On Postgres in-app backup is not offered at all (§16). An empty table would read as
  // "backups are broken" on the one install where the operator is correctly using pg_dump,
  // so the absence is explained instead of rendered.
  if (!list.supported) {
    return (
      <section className={cn("rounded-lg border border-border p-4", className)}>
        <div className="flex items-center gap-2">
          <Database className="size-4 text-muted-foreground" aria-hidden />
          <h2 className="font-medium text-sm">Backups</h2>
        </div>
        <p className="mt-2 text-muted-foreground text-sm">
          This install runs on PostgreSQL, which Loomarr does not back up itself. Use{" "}
          <code className="font-mono text-xs">pg_dump</code> against the database and restore with{" "}
          <code className="font-mono text-xs">pg_restore</code>, on whatever schedule you already run for your
          other databases.
        </p>
      </section>
    );
  }

  return (
    <section className={cn("rounded-lg border border-border p-4", className)}>
      <div className="flex items-start gap-4">
        <div className="min-w-0 flex-1">
          <h2 className="font-medium text-sm">Backups</h2>
          <p className="mt-1 text-muted-foreground text-sm leading-relaxed">
            A backup is the whole instance: settings, channels, people, and the generated secrets. Treat the
            file as a credential.
          </p>
        </div>
        <Button type="button" onClick={onBackUpNow} disabled={pending}>
          <HardDriveDownload className="size-4" aria-hidden />
          {pending ? "Backing up..." : "Back up now"}
        </Button>
      </div>

      {error ? (
        <p className="mt-3 flex items-start gap-2 text-danger text-sm" role="alert">
          <AlertTriangle className="mt-0.5 size-4 shrink-0" aria-hidden />
          {error}
        </p>
      ) : null}

      {backups.length === 0 ? (
        <p className="mt-4 rounded-md border border-border border-dashed p-4 text-center text-muted-foreground text-sm">
          No backups yet. One is written on the schedule below, or take the first one now.
        </p>
      ) : (
        <ul className="mt-4 flex flex-col gap-1.5">
          {backups.map((b) => (
            <li
              key={b.name}
              className="flex items-center gap-3 rounded-md border border-border bg-muted/30 px-3 py-2"
            >
              <span className="min-w-0 flex-1 truncate font-mono text-xs">{b.name}</span>
              <span className="shrink-0 font-mono text-muted-foreground text-xs">{formatBytes(b.bytes)}</span>
              <span className="shrink-0 font-mono text-muted-foreground text-xs">
                {/* writtenAt is Unix SECONDS, per the schema's epoch convention; formatRelative takes ms. */}
                {formatRelative(b.writtenAt * 1000)}
              </span>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => onDownload(b.name)}
                aria-label={`Download ${b.name}`}
              >
                <Download className="size-4" aria-hidden />
                Download
              </Button>
            </li>
          ))}
        </ul>
      )}

      <p className="mt-3 font-mono text-muted-foreground text-xs">
        {/* The raw cron rather than a friendly label: the default (03:30) matches no preset,
            so describeCron would render "Custom" — which tells the operator nothing. */}
        {list.schedule} into {list.dir} · keeps{" "}
        {list.retain > 0 ? pluralize(list.retain, "backup") : "every backup"} · restore is a command-line
        operation, deliberately
      </p>
    </section>
  );
};

export { BackupPanel };
