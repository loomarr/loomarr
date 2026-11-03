// PROTOTYPE — throwaway (#2151 step 2). The shared scan-health block, in plain words.
// "full" is the Settings → Library / wizard block; "compact" is the one calm sentence at the top
// of a Library page. The bar exists only while media is being added.
import { Action, Icon, icons, StatusDot, Surface, Text } from "@loomarr/design-system";
import type { ScanStatus } from "./prototype-data";
import { Callout, dotFor, Line, LitBar, pct, ReadyChips, Stack } from "./scan-health-parts";

interface ScanHealthProps {
  status: ScanStatus;
  // "full": Settings → Library and the wizard (sentence + bar + what keeps it up to date).
  // "compact": one sentence at the top of a Library page (plus a slim bar and one action).
  density: "full" | "compact";
  onOpenSettings?: () => void;
  // Inside the wizard the step owns "Check again" and the one primary (Continue), so the offline
  // callout states the fact without an action of its own.
  inFlow?: boolean;
}

const isFresh = (status: ScanStatus) => status.roots.every((r) => !r.confirmed);
const offlineRoots = (status: ScanStatus) => status.roots.filter((r) => !r.readable);

const Compact = ({ status, onOpenSettings }: Omit<ScanHealthProps, "density">) => {
  const { progress, held } = status;
  const offline = offlineRoots(status)[0];
  const fresh = isFresh(status);
  return (
    <Stack
      flexDirection="row"
      alignItems="center"
      flexWrap="wrap"
      columnGap="control"
      rowGap="inline"
      paddingVertical="inline"
    >
      <Line>
        <StatusDot
          tone={fresh ? "off" : held || offline ? "warn" : dotFor(status.tone)}
          label={status.headline}
        />
        <Text textRole="label">{status.headline}</Text>
      </Line>
      {progress ? (
        <Stack flexGrow={1} flexBasis={120} minWidth={96}>
          <LitBar
            percent={pct(progress)}
            label={`Adding your media, ${pct(progress)} percent`}
            paused={!!status.paused}
          />
        </Stack>
      ) : null}
      {held || offline ? <Action tone="primary">Check again</Action> : null}
      <Stack marginLeft="auto">
        <Action variant="ghost" onPress={onOpenSettings}>
          Library settings
        </Action>
      </Stack>
    </Stack>
  );
};

const Full = ({ status, inFlow }: Pick<ScanHealthProps, "status" | "inFlow">) => {
  const { held, paused, progress } = status;
  const offline = offlineRoots(status);
  const fresh = isFresh(status);
  // In the wizard the folder cards already say what's wrong; only progress belongs here.
  if (inFlow && !progress) return null;
  const showHeadline = inFlow ? !!progress : !held && offline.length === 0 && !fresh;
  return (
    <Surface padding="section" gap="section" aria-label="Library status" role="region">
      {showHeadline ? (
        <Line gap="control">
          <StatusDot tone={dotFor(status.tone)} label={status.headline} />
          <Text textRole="headline">{status.headline}</Text>
        </Line>
      ) : null}

      {held && !inFlow ? (
        <Callout
          tone="warn"
          title={`${held.wouldRemove.toLocaleString("en-US")} movies seem to be gone from your Movies folder`}
          actions={
            <>
              <Action tone="primary">Check again</Action>
              <Action tone="danger">They're gone, remove them</Action>
            </>
          }
        >
          We haven't removed anything — if a drive is unplugged, plug it back in.
        </Callout>
      ) : null}

      {inFlow
        ? null
        : offline.map((root) => (
            <Callout
              key={root.path}
              tone="warn"
              title="Can't reach your Movies folder"
              actions={<Action tone="primary">Check again</Action>}
            >
              Nothing has been removed. Channels skip those movies until it's back.
            </Callout>
          ))}

      {fresh && !inFlow ? (
        <Callout tone="warn" title="Tell Loomarr where your media is">
          Loomarr found these folders. Choose the ones that hold your library and it will start adding your
          media.
        </Callout>
      ) : null}

      {progress ? (
        <Stack gap="control">
          <LitBar
            percent={pct(progress)}
            label={`Adding your media, ${pct(progress)} percent`}
            paused={!!paused}
          />
          <Text textRole="body" tone="secondary">
            {paused ? "Picks up again when Saturday Morning is done." : "Titles appear as they're ready."}
          </Text>
          {paused ? null : <ReadyChips />}
        </Stack>
      ) : null}

      {fresh || inFlow ? null : (
        <Stack gap="control">
          <Text textRole="label" tone="secondary">
            Keeping up to date
          </Text>
          <Stack gap="inline">
            {status.live.map((signal) => (
              <Line key={signal.label} gap="inline" flexWrap="nowrap" alignItems="flex-start">
                {signal.ok ? (
                  <Icon decorative glyph={icons.success} size="compact" tone="success" />
                ) : (
                  <StatusDot tone="warn" label="Waiting" size="compact" />
                )}
                <Text textRole="body">
                  {signal.ok ? `${signal.label}` : `${signal.label} — waiting for Movies`}
                </Text>
              </Line>
            ))}
          </Stack>
        </Stack>
      )}
    </Surface>
  );
};

const ScanHealth = ({ status, density, onOpenSettings, inFlow }: ScanHealthProps) =>
  density === "compact" ? (
    <Compact status={status} onOpenSettings={onOpenSettings} />
  ) : (
    <Full status={status} inFlow={inFlow} />
  );

export type { ScanHealthProps };
export { ScanHealth };
