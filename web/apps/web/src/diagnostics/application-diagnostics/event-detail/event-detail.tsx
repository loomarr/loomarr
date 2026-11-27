import type { EventView } from "@loomarr/api/models/eventView";
import { Action, Card, Stack, StatusDot, Text, Tooltip } from "@loomarr/design-system";
import { useCopied } from "@/lib/use-copied";
import { CopyValue } from "../../copy-value";
import { fallbackMessage, formatTime, levelLabel, levelTone, sourceLabel } from "../event-format";

const EventDetail = ({
  event,
  onOpenProcess,
  onOpenRelated,
}: {
  event: EventView;
  onOpenProcess?: (id: string) => void;
  onOpenRelated?: (kind: "channel" | "job", id: string) => void;
}) => {
  const { copied, copy } = useCopied();
  const correlations = [
    ["Request id", event.requestId],
    ["Playback session", event.playbackSessionId],
    ["Channel", event.channelId],
    ["Schedule block", event.scheduleBlockId],
    ["Job", event.jobId],
    ["Process run", event.processRunId],
    ["Instance", event.instanceId],
  ].filter((entry): entry is [string, string] => Boolean(entry[1]));
  const copyLog = () => void copy(JSON.stringify(event, null, 2));
  return (
    <Stack className="bg-muted/20" gap="4" padding={16}>
      <Stack align="start" direction="row" gap="control" justify="between">
        <Stack gap="2">
          <Text as="span" textRole="hint" tone="secondary">
            <Stack align="center" direction="row" gap="2" render="span">
              <StatusDot tone={levelTone(event.level)} label={levelLabel(event.level)} />
              {levelLabel(event.level)} · {sourceLabel(event.source)}
            </Stack>
          </Text>
          <Stack gap="1">
            <Text as="h3" textRole="headline">
              {event.message || fallbackMessage(event)}
            </Text>
            <Text as="p" textRole="hint" tone="secondary">
              {formatTime(event.occurredAt)}
              {event.subsystem ? ` · ${event.subsystem}` : ""}
            </Text>
          </Stack>
        </Stack>
        <Tooltip content={copied ? "Copied" : "Copy complete log details"}>
          <Action
            icon={copied ? "success" : "copy"}
            layout="icon"
            onPress={copyLog}
            style={{ flexShrink: 0 }}
            variant="ghost"
          >
            {copied ? "Copied log details" : "Copy log details"}
          </Action>
        </Tooltip>
      </Stack>
      <Stack direction="row" gap="inline" wrap>
        {event.processRunId && onOpenProcess && (
          <Action icon="process" onPress={() => onOpenProcess(event.processRunId ?? "")} variant="outline">
            Open process output
          </Action>
        )}
        {event.channelId && onOpenRelated && (
          <Action onPress={() => onOpenRelated("channel", event.channelId ?? "")} variant="outline">
            Open related Channel
          </Action>
        )}
        {event.jobId && onOpenRelated && (
          <Action onPress={() => onOpenRelated("job", event.jobId ?? "")} variant="outline">
            Open related Job
          </Action>
        )}
      </Stack>
      <Card padding={12} render="details">
        <Text as="summary" cursor="pointer" textRole="cardLabel">
          Technical details
        </Text>
        <Stack gap="3" marginTop={12}>
          <Text as="p" textRole="mono">
            {event.event}
          </Text>
          {correlations.length > 0 && (
            <Stack gap="1">
              {correlations.map(([label, value]) => (
                <Stack align="center" direction="row" flexShrink={1} gap="1" key={label} minWidth={0}>
                  <Stack flexShrink={0} render="span" width={112}>
                    <Text as="span" textRole="hint" tone="secondary">
                      {label}
                    </Text>
                  </Stack>
                  <CopyValue label={label} value={value} />
                </Stack>
              ))}
            </Stack>
          )}
          {Object.keys(event.attributes).length === 0 ? (
            <Text as="p" textRole="hint" tone="secondary">
              No structured attributes.
            </Text>
          ) : (
            <Card maxHeight={224} overflow="auto" padding={12}>
              <Text as="pre" textRole="mono" tone="primary">
                {JSON.stringify(event.attributes, null, 2)}
              </Text>
            </Card>
          )}
        </Stack>
      </Card>
    </Stack>
  );
};

export { EventDetail };
