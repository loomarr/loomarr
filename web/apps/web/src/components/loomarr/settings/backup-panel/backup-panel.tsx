import { formatBytes, formatRelative, pluralize } from "@loomarr/core/format";
import { Action, Card, Icon, icons, Stack, Surface, Text } from "@loomarr/design-system";
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
      <Card className={className} gap={8} padding={16} render="section">
        <Stack align="center" direction="row" gap="inline">
          <Icon decorative glyph={icons.database} size="compact" tone="muted" />
          <Text as="h2" textRole="cardLabel">
            Backups
          </Text>
        </Stack>
        <Text as="p" textRole="compact" tone="secondary">
          This install runs on PostgreSQL, which Loomarr does not back up itself. Use{" "}
          <Text as="code" textRole="mono">
            pg_dump
          </Text>{" "}
          against the database and restore with{" "}
          <Text as="code" textRole="mono">
            pg_restore
          </Text>
          , on whatever schedule you already run for your other databases.
        </Text>
      </Card>
    );
  }

  return (
    <Card className={className} padding={16} render="section">
      <Stack align="start" direction="row" gap="4">
        <Stack flexBasis="0%" flexGrow={1} flexShrink={1} gap="1" minWidth={0}>
          <Text as="h2" textRole="cardLabel">
            Backups
          </Text>
          <Text as="p" textRole="compact" tone="secondary">
            A backup is the whole instance: settings, channels, people, and the generated secrets. Treat the
            file as a credential.
          </Text>
        </Stack>
        <Action disabled={pending} icon="download" onPress={onBackUpNow}>
          {pending ? "Backing up..." : "Back up now"}
        </Action>
      </Stack>

      {error ? (
        <Stack align="start" direction="row" gap="inline" marginTop={12} render="p" role="alert">
          <Stack direction="row" marginTop={2} render="span">
            <Icon decorative glyph={icons.alert} size="compact" tone="danger" />
          </Stack>
          <Text as="span" textRole="compact" tone="danger">
            {error}
          </Text>
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
        <Stack
          borderColor="borderDecorative"
          borderRadius={8}
          borderStyle="dashed"
          borderWidth={1}
          marginTop={16}
          padding={16}
        >
          <Text as="p" className="text-center" textRole="compact" tone="secondary">
            No backups yet. One is written on the schedule below, or take the first one now.
          </Text>
        </Stack>
      ) : (
        <Stack gap="1.5" marginTop={16} render="ul">
          {backups.map((b) => (
            <Stack
              align="center"
              borderColor="borderDecorative"
              borderRadius={8}
              borderWidth={1}
              className="bg-muted/30"
              direction="row"
              gap="control"
              key={b.name}
              paddingHorizontal={12}
              paddingVertical={8}
              render="li"
            >
              <Text as="span" className="min-w-0 flex-1 truncate" textRole="mono" tone="primary">
                {b.name}
              </Text>
              <Text flexShrink={0} textRole="mono">
                {formatBytes(b.bytes)}
              </Text>
              <Text flexShrink={0} textRole="mono">
                {/* writtenAt is Unix SECONDS, per the schema's epoch convention; formatRelative takes ms. */}
                {formatRelative(b.writtenAt * 1000)}
              </Text>
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
        </Stack>
      )}

      <Stack marginTop={12}>
        <Text as="p" textRole="mono">
          {/* The raw cron rather than a friendly label: the default (03:30) matches no preset,
              so describeCron would render "Custom" — which tells the operator nothing. */}
          {list.schedule} into {list.dir} · keeps{" "}
          {list.retain > 0 ? pluralize(list.retain, "backup") : "every backup"} · restore is a command-line
          operation
        </Text>
      </Stack>
    </Card>
  );
};

export { BackupPanel };
