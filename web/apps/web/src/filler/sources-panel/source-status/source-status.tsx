import type { FetchFillerSourceOutputBody } from "@loomarr/api/models/fetchFillerSourceOutputBody";
import type { FillerSourceDTO } from "@loomarr/api/models/fillerSourceDTO";
import { formatRelative } from "@loomarr/core/format";
import { Action, Stack, Text, TextLink } from "@loomarr/design-system";
import { useState } from "react";

const checkResultText = (source: FillerSourceDTO, result: FetchFillerSourceOutputBody) => {
  const limit = ` This check could add up to ${result.maxPerCheck} ${result.maxPerCheck === 1 ? "clip" : "clips"}.`;
  const skipped =
    result.skipped > 0
      ? ` ${result.skipped} ${result.skipped === 1 ? "clip was" : "clips were"} already known and skipped.`
      : "";
  if (result.stoppedBy === "catalog") {
    return `Couldn’t add clips from ${source.target} because the clip limit is full.${limit}`;
  }
  if (result.queued > 0) {
    return `${result.queued} new ${result.queued === 1 ? "clip was" : "clips were"} queued from ${source.target}.${limit}${skipped}`;
  }
  if (result.added > 0 || result.updated > 0) {
    return `Found ${result.added} new and ${result.updated} updated ${result.added + result.updated === 1 ? "clip" : "clips"} in ${source.target}.`;
  }
  return `No new clips were found in ${source.target}.${limit}${skipped}`;
};

const readinessLabel = (source: FillerSourceDTO) => {
  switch (source.readiness) {
    case "off":
    case "provider_off":
      return "Paused";
    case "needs_location":
      return "Location needed";
    case "out_of_area":
      return "Not used in your area";
    case "unavailable":
      return "Couldn’t check this source";
    case "not_configured":
      return "Setup needed";
    default:
      return "Ready";
  }
};

interface SourceStatusProps {
  source: FillerSourceDTO;
  fetching: boolean;
  /** The failed fetch, if any. Shared across sources, so it can describe an earlier selection. */
  fetchError?: { detail?: string } | null;
  /** Starts a check; `onResult` receives what it found. */
  onFetch: (sourceID: string, onResult: (result: FetchFillerSourceOutputBody | undefined) => void) => void;
}

// The top of the source drawer: whether the source is usable, how many clips it holds and when it was
// last checked, plus "Look for new clips" and the plain-language outcome of the check that follows.
const SourceStatus = ({ source, fetching, fetchError, onFetch }: SourceStatusProps) => {
  const [checkResult, setCheckResult] = useState<FetchFillerSourceOutputBody>();

  return (
    <Stack aria-labelledby="source-status-heading" gap="control" render="section">
      <div>
        <Text as="h3" id="source-status-heading" textRole="heading">
          {readinessLabel(source)}
        </Text>
        <Stack marginTop={4}>
          <Text as="p" textRole="compact" tone="secondary">
            {source.detail}
            {source.onForPlace &&
              source.enabled &&
              " Loomarr switched this on because it covers TV stations near your location. If you switch it off, it stays off."}
          </Text>
        </Stack>
        <Stack marginTop={4}>
          <Text as="p" textRole="hint" tone="secondary">
            {source.incoming > 0
              ? `${source.count} ready · `
              : `${source.count} ${source.count === 1 ? "clip" : "clips"}`}
            {source.incoming > 0 && (
              <TextLink href="/filler/incoming">{`${source.incoming} being checked`}</TextLink>
            )}
            {source.lastCheckedAt
              ? ` · last checked ${formatRelative(source.lastCheckedAt)}`
              : " · not checked yet"}
          </Text>
        </Stack>
        {source.kind === "folder" && (
          <Stack marginTop={12}>
            <Text as="p" className="break-all" textRole="mono">
              {source.uri ?? source.target}
            </Text>
          </Stack>
        )}
      </div>

      {source.actions.includes("fetch") && (
        <Action
          disabled={fetching}
          onPress={() => {
            setCheckResult(undefined);
            onFetch(source.id, setCheckResult);
          }}
          style={{ alignSelf: "flex-start" }}
        >
          {fetching ? `Looking in ${source.target}…` : "Look for new clips"}
        </Action>
      )}

      {checkResult && checkResult.sourceId === source.id && (
        <Stack
          borderRadius={6}
          className="bg-muted/40"
          paddingHorizontal={12}
          paddingVertical={8}
          role="status"
        >
          <Text as="p" textRole="compact">
            {checkResultText(source, checkResult)}
          </Text>
        </Stack>
      )}
      {fetchError && (
        <Stack
          borderRadius={6}
          className="bg-destructive/10"
          paddingHorizontal={12}
          paddingVertical={8}
          role="alert"
        >
          <Text as="p" textRole="compact" tone="danger">
            {fetchError.detail}
          </Text>
        </Stack>
      )}
    </Stack>
  );
};

export type { SourceStatusProps };
export { checkResultText, readinessLabel, SourceStatus };
