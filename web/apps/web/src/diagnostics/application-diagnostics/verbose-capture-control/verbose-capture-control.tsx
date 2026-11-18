import * as diagnosticsApi from "@loomarr/api/endpoints/diagnostics";
import { Action, Card, CompactSelect, Icon, icons, Stack, Text } from "@loomarr/design-system";
import { useEffect, useState } from "react";
import type { ApplicationFilters } from "../../application-filters";
import { formatTime } from "../event-format";

const VerboseCaptureControl = ({ filters }: { filters: ApplicationFilters }) => {
  const [duration, setDuration] = useState("5");
  // A start or stop anywhere reads this again on the `diagnostic_events` frame (#3352). A capture
  // that runs out stores nothing, so the read is repeated at its end instead.
  const query = diagnosticsApi.useGetDiagnosticVerboseCapture({ query: { retry: false } });
  const start = diagnosticsApi.useStartDiagnosticVerboseCapture();
  const stop = diagnosticsApi.useStopDiagnosticVerboseCapture();
  const capture = query.data?.status === 200 ? query.data.data : undefined;
  const endsAt = capture?.active ? capture.endsAt : undefined;
  const { refetch } = query;
  useEffect(() => {
    if (endsAt === undefined) return;
    const timer = setTimeout(() => void refetch(), Math.max(0, endsAt - Date.now()));
    return () => clearTimeout(timer);
  }, [endsAt, refetch]);
  const busy = start.isPending || stop.isPending;
  const startCapture = async () => {
    await start.mutateAsync({
      data: {
        durationMinutes: Number(duration),
        ...(filters.subsystem.trim() ? { subsystem: filters.subsystem.trim() } : {}),
        ...(filters.channelId ? { channelId: filters.channelId } : {}),
      },
    });
    await query.refetch();
  };
  const stopCapture = async () => {
    await stop.mutateAsync();
    await query.refetch();
  };
  return (
    <Card aria-labelledby="verbose-capture-title" gap={8} padding={12} render="section">
      <Stack align="center" direction="row" gap="control" wrap>
        <Stack flexBasis="0%" flexGrow={1} minWidth={256}>
          <Text as="h3" id="verbose-capture-title" textRole="cardLabel">
            Verbose capture
          </Text>
          <Text as="p" textRole="hint" tone="secondary">
            Temporarily retain debug events
            {filters.subsystem.trim() ? ` for ${filters.subsystem.trim()}` : ""}
            {filters.channelId ? ` on Channel ${filters.channelId}` : ""}. The same redaction, queue, and
            retention limits still apply.
          </Text>
        </Stack>
        {capture?.active ? (
          <>
            <Text as="span" textRole="compact">
              <Stack align="center" direction="row" gap="2" render="span">
                <Icon decorative glyph={icons.capture} size="compact" tone="warning" />
                Active until {formatTime(capture.endsAt ?? Date.now())}
              </Stack>
            </Text>
            <Action disabled={busy} icon="stop" onPress={() => void stopCapture()} variant="outline">
              Stop
            </Action>
          </>
        ) : (
          <>
            <CompactSelect
              label="Verbose capture duration"
              onValueChange={setDuration}
              options={[
                { value: "1", label: "1 minute" },
                { value: "5", label: "5 minutes" },
                { value: "10", label: "10 minutes" },
                { value: "15", label: "15 minutes" },
              ]}
              value={duration}
              width={128}
            />
            <Action
              disabled={busy || query.isError}
              icon="capture"
              onPress={() => void startCapture()}
              variant="outline"
            >
              Start capture
            </Action>
          </>
        )}
      </Stack>
      {query.isError && (
        <Text as="p" textRole="hint" tone="danger">
          Verbose capture is unavailable.
        </Text>
      )}
      {(start.isError || stop.isError) && (
        <Text as="p" textRole="hint" tone="danger">
          The capture control could not be updated.
        </Text>
      )}
    </Card>
  );
};

export { VerboseCaptureControl };
