import { formatBytes, formatRelative, pluralize } from "@loomarr/core/format";
import { Action, Icon, icons, Stack, Surface, Text } from "@loomarr/design-system";
import { cn } from "@/lib/utils";
import type { BackupPanelProps } from "./backup-panel.type";
import { missingNights } from "./missing-nights";

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

const BackupPanel = ({
  list,
  onBackUpNow,
  onDownload,
  onOpenTasks,
  pending,
  error,
  className,
}: BackupPanelProps) => {
  const backups = list.backups ?? [];
  const missing = missingNights(backups);

  // On Postgres in-app backup is not offered at all (§16). An empty table would read as
  // "backups are broken" on the one install where the operator is correctly using pg_dump,
  // so the absence is explained instead of rendered.
  if (!list.supported) {
    return (
      <section className={cn("border-border rounded-lg border p-4", className)}>
        <Stack align="center" direction="row" gap="inline">
          <Icon decorative glyph={icons.database} size="compact" tone="muted" />
          <h2 className="text-sm font-medium">Backups</h2>
        </Stack>
        <p className="text-muted-foreground mt-2 text-sm">
          This install runs on PostgreSQL, which Loomarr does not back up itself. Use{" "}
          <code className="font-mono text-xs">pg_dump</code> against the database and restore with{" "}
          <code className="font-mono text-xs">pg_restore</code>, on whatever schedule you already run for your
          other databases.
        </p>
      </section>
    );
  }

  return (
    <section className={cn("border-border rounded-lg border p-4", className)}>
      <div className="flex items-start gap-4">
        <div className="min-w-0 flex-1">
          <h2 className="text-sm font-medium">Backups</h2>
          <p className="text-muted-foreground mt-1 text-sm leading-relaxed">
            A backup is the whole instance: settings, channels, people, and the generated secrets. Treat the
            file as a credential.
          </p>
        </div>
        <Action disabled={pending} icon="download" onPress={onBackUpNow}>
          {pending ? "Backing up..." : "Back up now"}
        </Action>
      </div>

      {error ? (
        <Stack
          align="start"
          className="text-danger mt-3 text-sm"
          direction="row"
          gap="inline"
          render="p"
          role="alert"
        >
          <Stack className="mt-0.5" direction="row" render="span">
            <Icon decorative glyph={icons.alert} size="compact" tone="inherit" />
          </Stack>
          {error}
        </Stack>
      ) : null}

      {missing > 0 ? (
        <Surface
          alignItems="center"
          backgroundColor="transparent"
          borderWidth={0}
          flexDirection="row"
          flexWrap="wrap"
          gap="inline"
          marginTop={12}
          role="status"
        >
          <Icon decorative glyph={icons.warning} size="compact" tone="warning" />
          <Text textRole="body" tone="warning">
            {pluralize(missing, "night")} missing from the nightly backups.
          </Text>
          {onOpenTasks ? (
            <Action onPress={onOpenTasks} variant="ghost">
              Open Tasks
            </Action>
          ) : null}
        </Surface>
      ) : null}

      {backups.length === 0 ? (
        <p className="border-border text-muted-foreground mt-4 rounded-md border border-dashed p-4 text-center text-sm">
          No backups yet. One is written on the schedule below, or take the first one now.
        </p>
      ) : (
        <ul className="mt-4 flex flex-col gap-1.5">
          {backups.map((b) => (
            <Stack
              align="center"
              className="border-border bg-muted/30 rounded-md border px-3 py-2"
              direction="row"
              gap="control"
              key={b.name}
              render="li"
            >
              <span className="min-w-0 flex-1 truncate font-mono text-xs">{b.name}</span>
              <span className="text-muted-foreground shrink-0 font-mono text-xs">{formatBytes(b.bytes)}</span>
              <span className="text-muted-foreground shrink-0 font-mono text-xs">
                {/* writtenAt is Unix SECONDS, per the schema's epoch convention; formatRelative takes ms. */}
                {formatRelative(b.writtenAt * 1000)}
              </span>
              <Action
                accessibilityLabel={`Download ${b.name}`}
                icon="download"
                onPress={() => onDownload(b.name)}
                variant="outline"
              >
                Download
              </Action>
            </Stack>
          ))}
        </ul>
      )}

      <p className="text-muted-foreground mt-3 font-mono text-xs">
        {/* The raw cron rather than a friendly label: the default (03:30) matches no preset,
            so describeCron would render "Custom" — which tells the operator nothing. */}
        {list.schedule} into {list.dir} · keeps{" "}
        {list.retain > 0 ? pluralize(list.retain, "backup") : "every backup"} · restore is a command-line
        operation
      </p>
    </section>
  );
};

export { BackupPanel };
