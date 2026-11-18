import { Action, Surface, Text, Toggle } from "@loomarr/design-system";
import type { ReactNode } from "react";

import type { ChannelGrowthSectionProps } from "./channel-growth-section.type";
import { groupByNight, shortfallWords, stillWords, waitingWords } from "./growth-words";

// GROWS BY ITSELF — the channel page's plain growth controls (#4333, decision 25 of #4140): what the
// channel adds, why it isn't growing when it isn't, what it still needs, what joined by itself (each
// with a way to take it off) and what was taken back off (each with a way to add it back).
// Presentational: the app owns the data and the saves. No designed grid or line editor yet.

const Row = ({ children }: { children: ReactNode }) => (
  <Surface
    alignItems="center"
    backgroundColor="transparent"
    borderWidth={0}
    flexDirection="row"
    gap={12}
    justifyContent="space-between"
    role="listitem"
  >
    {children}
  </Surface>
);

const ChannelGrowthSection = ({
  busy = false,
  joined,
  keep,
  lineWords,
  off,
  onAddBack,
  onKeepChange,
  onTakeOff,
  short,
  still,
  target,
  waiting,
}: ChannelGrowthSectionProps) => {
  const why = stillWords(still);
  const shortfall = shortfallWords(target, short);
  const waitingLine = waitingWords(waiting);
  const nights = groupByNight(joined);
  return (
    <Surface aria-label="Grows by itself" backgroundColor="surfaceRaised" gap={20} padding={20} role="region">
      <Surface backgroundColor="transparent" borderWidth={0} gap={4}>
        <Text textRole="title">Grows by itself</Text>
        <Text textRole="hint" tone="muted">
          Titles from your library that fit this channel's picks join it on their own.
        </Text>
      </Surface>

      {lineWords ? (
        <Surface backgroundColor="transparent" borderWidth={0} gap={4}>
          <Text textRole="label">What it adds</Text>
          <Text textRole="body">{lineWords}</Text>
        </Surface>
      ) : null}

      {why ? <Text textRole="body">{why}</Text> : null}
      {shortfall ? <Text textRole="body">{shortfall}</Text> : null}
      {waitingLine ? <Text textRole="body">{waitingLine}</Text> : null}

      <Toggle
        checked={keep}
        description="Stops new titles from joining. What is already here stays."
        disabled={busy}
        kind="switch"
        label="Keep as it is"
        onCheckedChange={onKeepChange}
      />

      {nights.length > 0 ? (
        <Surface backgroundColor="transparent" borderWidth={0} gap={12}>
          <Text textRole="label">Joined by itself</Text>
          {nights.map((night) => (
            <Surface
              aria-label={night.label}
              backgroundColor="transparent"
              borderWidth={0}
              gap={8}
              key={night.label}
              role="list"
            >
              <Text textRole="hint" tone="muted">
                {night.label}
              </Text>
              {night.titles.map((title) => (
                <Row key={title.key}>
                  <Text textRole="body">{title.name}</Text>
                  <Action
                    accessibilityLabel={`Take off ${title.name}`}
                    disabled={busy}
                    onPress={() => onTakeOff(title.key)}
                    variant="outline"
                  >
                    Take off
                  </Action>
                </Row>
              ))}
            </Surface>
          ))}
        </Surface>
      ) : null}

      {off.length > 0 ? (
        <Surface
          aria-label="Taken back off"
          backgroundColor="transparent"
          borderWidth={0}
          gap={8}
          role="list"
        >
          <Text textRole="label">Taken back off</Text>
          {off.map((title) => (
            <Row key={title.key}>
              <Surface backgroundColor="transparent" borderWidth={0} gap={2}>
                <Text textRole="body">{title.name}</Text>
                <Text textRole="hint" tone="muted">
                  {title.reason || "Taken off by hand"}
                </Text>
              </Surface>
              <Action
                accessibilityLabel={`Add back ${title.name}`}
                disabled={busy}
                onPress={() => onAddBack(title.key)}
                variant="outline"
              >
                Add back
              </Action>
            </Row>
          ))}
        </Surface>
      ) : null}
    </Surface>
  );
};

export { ChannelGrowthSection };
