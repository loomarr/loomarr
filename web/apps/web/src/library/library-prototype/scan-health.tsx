// PROTOTYPE — throwaway (#2151 step 2). The shared scan-health strip + stage stepper (decision 34).
// "full" is the Settings → Library / wizard health block; "compact" is the one line that sits at
// the top of a Library page. The bar exists only while a scan is running; idle shows none.
import { Action, ProgressTrack, StatusDot, Surface, Text } from "@loomarr/design-system";
import type { ScanStatus } from "./prototype-data";
import { Callout, dotFor, fmt, Line, pct, Stack, StageStepper } from "./scan-health-parts";

interface ScanHealthProps {
  status: ScanStatus;
  // "full": Settings → Library and the wizard (stepper + bar + live signals).
  // "compact": one line at the top of a Library page (headline + bar while running + action).
  density: "full" | "compact";
  onOpenSettings?: () => void;
}

const isFresh = (status: ScanStatus) => status.roots.every((r) => !r.confirmed);
const offlineRoots = (status: ScanStatus) => status.roots.filter((r) => !r.readable);

const Compact = ({ status, onOpenSettings }: Omit<ScanHealthProps, "density">) => {
  const { progress, held, paused } = status;
  const offline = offlineRoots(status)[0];
  const fresh = isFresh(status);
  const headline =
    status.tone === "ok" ? `${status.headline} · last change ${status.lastChange}` : status.headline;
  return (
    <Surface
      flexDirection="row"
      alignItems="center"
      flexWrap="wrap"
      columnGap="control"
      rowGap="inline"
      paddingHorizontal="control"
      paddingVertical="inline"
      borderRadius="control"
      borderColor={held || fresh ? "stateWarning" : offline ? "stateDanger" : "borderDecorative"}
    >
      <Line>
        <StatusDot tone={dotFor(status.tone)} label={`Library scan: ${status.headline}`} />
        <Text textRole="label">{headline}</Text>
      </Line>
      {progress ? (
        <Line gap="control" flexGrow={1} minWidth={200}>
          <Stack flexGrow={1} flexBasis={120} minWidth={96}>
            <ProgressTrack
              width="100%"
              percent={pct(progress)}
              label={`Library scan ${pct(progress)} percent`}
            />
          </Stack>
          <Text textRole="metadata" tone="secondary">
            {fmt(progress.done)} / {fmt(progress.total)} · {progress.eta}
          </Text>
        </Line>
      ) : null}
      {paused ? (
        <Text textRole="metadata" tone="secondary">
          {paused}
        </Text>
      ) : null}
      {held ? (
        <Line gap="control">
          <Text textRole="metadata" tone="warning">
            Nothing removed yet
          </Text>
          <Action variant="suggest">Confirm removal</Action>
        </Line>
      ) : null}
      {offline ? (
        <Line gap="control">
          <Text textRole="metadata" tone="danger">
            Nothing marked missing
          </Text>
          <Action variant="outline">Re-check</Action>
        </Line>
      ) : null}
      {fresh ? (
        <Action variant="suggest" onPress={onOpenSettings}>
          Confirm roots
        </Action>
      ) : null}
      <Stack marginLeft="auto">
        <Action variant="link" onPress={onOpenSettings}>
          Settings
        </Action>
      </Stack>
    </Surface>
  );
};

const Full = ({ status }: Pick<ScanHealthProps, "status">) => {
  const { held, paused } = status;
  const offline = offlineRoots(status);
  const fresh = isFresh(status);
  return (
    <Surface padding="section" gap="control" aria-label="Library scan health" role="region">
      <Line gap="control">
        <StatusDot tone={dotFor(status.tone)} label={`Library scan: ${status.headline}`} />
        <Text textRole="headline">{status.headline}</Text>
      </Line>

      {held ? (
        <Callout
          tone="warn"
          title={`${fmt(held.wouldRemove)} movies in ${held.root} look removed — nothing has been removed yet`}
          actions={
            <>
              <Action variant="suggest">Confirm removal</Action>
              <Action variant="outline">Rescan</Action>
            </>
          }
        >
          A walk finished {held.since} and found none of the {fmt(held.of)} titles under {held.root}. If the
          share is just empty or half-mounted, rescan after fixing it; if you really moved them, confirm.
        </Callout>
      ) : null}

      {offline.map((root) => (
        <Callout
          key={root.path}
          tone="bad"
          title={`${root.path} is offline — nothing is marked missing; channels skip its items until it returns`}
          actions={<Action variant="outline">Re-check</Action>}
        >
          Loomarr will pick the files up again as soon as the folder is readable.
        </Callout>
      ))}

      {fresh ? (
        <Callout tone="warn" title="Confirm where your media is">
          Loomarr found these folders. Nothing is imported until you confirm them in the scan roots below.
        </Callout>
      ) : null}

      <StageStepper status={status} />

      {paused ? (
        <Text textRole="body" tone="secondary">
          Paused {paused}
        </Text>
      ) : null}

      <Line columnGap="section" rowGap="inline">
        <Text textRole="metadata" tone="secondary">
          {fmt(status.totalFiles)} files
        </Text>
        <Text textRole="metadata" tone="secondary">
          {fmt(status.totalTitles)} titles
        </Text>
        <Text textRole="metadata" tone="secondary">
          last walk {status.lastWalk}
        </Text>
        <Text textRole="metadata" tone="secondary">
          last change {status.lastChange}
        </Text>
        <Action variant="link">{`unmatched ${status.unmatched} →`}</Action>
      </Line>

      <Stack gap="inline">
        <Text textRole="section" shout tone="muted">
          Live changes
        </Text>
        <Line columnGap="section" rowGap="inline">
          {status.live.map((signal) => (
            <Line key={signal.label}>
              <StatusDot
                tone={signal.ok ? "ok" : "error"}
                label={signal.ok ? "working" : "not working"}
                size="compact"
              />
              <Text textRole="metadata">
                {signal.label} {signal.ok ? "✓" : "✗"}
              </Text>
              <Text textRole="metadata" tone="muted">
                {signal.detail}
              </Text>
            </Line>
          ))}
        </Line>
      </Stack>
    </Surface>
  );
};

const ScanHealth = ({ status, density, onOpenSettings }: ScanHealthProps) =>
  density === "compact" ? (
    <Compact status={status} onOpenSettings={onOpenSettings} />
  ) : (
    <Full status={status} />
  );

export type { ScanHealthProps };
export { ScanHealth };
