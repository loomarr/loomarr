// PROTOTYPE — throwaway (#2151 step 2). Variant B ("Split view") shared bits: small DS-built
// pieces every pane of the master–detail page reuses.
import {
  Action,
  Badge,
  type BadgeTone,
  type IconName,
  StatusDot,
  type StatusTone,
  Surface,
  Text,
} from "@loomarr/design-system";
import type { ComponentProps, ReactNode } from "react";
import { Pressable } from "react-native";
import type { FactsState, IdentitySource, Provider } from "./prototype-data";

// A transparent layout box: Surface without its card chrome.
const Box = (props: ComponentProps<typeof Surface>) => (
  <Surface backgroundColor="transparent" borderWidth={0} {...props} />
);

// A boxed block inside the detail pane.
const Panel = ({ children, ...props }: ComponentProps<typeof Surface>) => (
  <Surface gap="inline" level="raised" padding="control" {...props}>
    {children}
  </Surface>
);

const Meta = ({
  children,
  tone = "secondary",
}: {
  children: ReactNode;
  tone?: "secondary" | "muted" | "warning" | "danger" | "success";
}) => (
  <Text textRole="metadata" tone={tone}>
    {children}
  </Text>
);

const SectionLabel = ({ children }: { children: ReactNode }) => (
  <Text shout textRole="metadata" tone="secondary">
    {children}
  </Text>
);

// Hue-lit artwork stand-in (never a remote image).
const Art = ({
  hue,
  title,
  width = 32,
  ratio = 1.5,
}: {
  hue: number;
  title: string;
  width?: number;
  ratio?: number;
}) => (
  <Surface
    alignItems="center"
    backgroundColor={`hsl(${hue} 38% 24%)`}
    borderRadius={4}
    flexShrink={0}
    height={Math.round(width * ratio)}
    justifyContent="center"
    width={width}
  >
    <Text textRole={width >= 56 ? "title" : "metadata"} tone="secondary">
      {title.charAt(0)}
    </Text>
  </Surface>
);

const FACTS: Record<
  FactsState,
  { tone: StatusTone; label: string; text: "secondary" | "success" | "danger" | "muted" }
> = {
  waiting: { tone: "off", label: "waiting", text: "muted" },
  reading: { tone: "progress", label: "reading", text: "secondary" },
  read: { tone: "ok", label: "✓ read", text: "success" },
  measured: { tone: "ok", label: "✓ measured", text: "success" },
  failed: { tone: "error", label: "! failed", text: "danger" },
};

// Facts state: waiting · reading · ✓ read · ✓ measured · ! failed (a series adds its x/y count).
const FactsMark = ({ facts, detail }: { facts: FactsState; detail?: string }) => {
  const f = FACTS[facts];
  const settled = facts === "read" || facts === "measured";
  const text = detail ? (settled ? `✓ ${detail}` : `${f.label} · ${detail}`) : f.label;
  return (
    <Box alignItems="center" flexDirection="row" flexShrink={0} gap={6}>
      <StatusDot label="" size="compact" tone={f.tone} />
      <Text numberOfLines={1} textRole="metadata" tone={f.text}>
        {text}
      </Text>
    </Box>
  );
};

const IDENTITY_TONE: Record<IdentitySource, BadgeTone> = {
  Sonarr: "info",
  Radarr: "info",
  NFO: "neutral",
  Linked: "override",
  "Embedded id": "neutral",
};
const IdentityChip = ({ source }: { source: IdentitySource }) => (
  <Badge tone={IDENTITY_TONE[source]}>{source}</Badge>
);

// A field's source chip; pressable when there is provenance to open.
const SourceChip = ({
  source,
  onPress,
  active,
}: {
  source: Provider;
  onPress?: () => void;
  active?: boolean;
}) =>
  onPress ? (
    <Pressable
      accessibilityLabel={`Why this value: source ${source}`}
      accessibilityRole="button"
      onPress={onPress}
    >
      <Badge tone={active ? "info" : "neutral"}>{`${source} ▾`}</Badge>
    </Pressable>
  ) : (
    <Badge tone="neutral">{source}</Badge>
  );

const Chip = ({ label, selected, onPress }: { label: string; selected: boolean; onPress: () => void }) => (
  <Action onPress={onPress} selected={selected} tone="secondary" variant="outline">
    {label}
  </Action>
);

const EmptyNote = ({
  title,
  body,
  action,
}: {
  title: string;
  body: string;
  action?: { label: string; icon?: IconName; onPress: () => void };
}) => (
  <Panel alignItems="flex-start" gap="control" padding="section">
    <Text textRole="label">{title}</Text>
    <Text textRole="body" tone="secondary">
      {body}
    </Text>
    {action ? (
      <Action icon={action.icon} onPress={action.onPress}>
        {action.label}
      </Action>
    ) : null}
  </Panel>
);

// A master-list row: pressable, two lines, the selected one raised.
const ListRow = ({
  selected,
  onPress,
  label,
  children,
}: {
  selected: boolean;
  onPress: () => void;
  label: string;
  children: ReactNode;
}) => (
  <Pressable
    accessibilityLabel={label}
    accessibilityRole="button"
    accessibilityState={{ selected }}
    onPress={onPress}
  >
    <Surface
      alignItems="center"
      backgroundColor={selected ? "surfaceElevated" : "transparent"}
      borderColor={selected ? "borderControl" : "borderDecorative"}
      borderRadius={8}
      flexDirection="row"
      gap="control"
      paddingHorizontal="control"
      paddingVertical={8}
    >
      {children}
    </Surface>
  </Pressable>
);

export { Art, Box, Chip, EmptyNote, FactsMark, IdentityChip, ListRow, Meta, Panel, SectionLabel, SourceChip };
