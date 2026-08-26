import { Action, ActivityIndicator, Icon, icons, Surface, Text } from "@loomarr/design-system";

import type { StatePanelKind, StatePanelProps } from "./state-panel.type";

const statePresentation: Record<
  Exclude<StatePanelKind, "loading">,
  {
    icon: typeof icons.info;
    iconLabel: string;
    tone: "danger" | "info" | "muted" | "warning";
  }
> = {
  empty: { icon: icons.info, iconLabel: "No content", tone: "muted" },
  error: { icon: icons.warning, iconLabel: "Error", tone: "danger" },
  offline: { icon: icons.warning, iconLabel: "Offline", tone: "warning" },
  permission: { icon: icons.warning, iconLabel: "Permission required", tone: "warning" },
};

const StatePanel = ({
  action,
  compact = false,
  description,
  density = "pointer",
  icon,
  kind,
  metadata,
  title,
}: StatePanelProps) => {
  const loading = kind === "loading";
  const presentation = loading ? undefined : statePresentation[kind];

  return (
    <Surface
      aria-busy={loading || undefined}
      aria-live={kind === "error" ? "assertive" : "polite"}
      alignItems="center"
      gap={compact ? "$inline" : "$control"}
      justifyContent="center"
      level="raised"
      minHeight={compact ? (density === "tv" ? 220 : 180) : density === "tv" ? 320 : 220}
      padding={compact ? "$control" : density === "tv" ? 48 : "$section"}
      role={kind === "error" ? "alert" : "status"}
      width="100%"
    >
      {icon ??
        (loading ? (
          <Surface alignItems="center" backgroundColor="$transparent" borderWidth={0}>
            <ActivityIndicator accessibilityLabel={title} size={density === "tv" ? "tv" : "control"} />
          </Surface>
        ) : (
          <Icon
            accessibilityLabel={presentation?.iconLabel ?? title}
            glyph={presentation?.icon ?? icons.info}
            size={density === "tv" ? "tv" : "control"}
            tone={presentation?.tone ?? "info"}
          />
        ))}
      <Surface
        alignItems="center"
        backgroundColor="$transparent"
        borderWidth={0}
        gap="$inline"
        maxWidth={density === "tv" ? 880 : 560}
      >
        <Text density={density} textAlign="center" textRole="title">
          {title}
        </Text>
        {description ? (
          <Text density={density} textAlign="center" textRole="body">
            {description}
          </Text>
        ) : null}
      </Surface>
      {action ? (
        <Action
          accessibilityRole="button"
          density={density}
          hasTVPreferredFocus={density === "tv"}
          onPress={action.onPress}
        >
          {action.label}
        </Action>
      ) : null}
      {metadata ? (
        <Text density={density} textAlign="center" textRole="metadata">
          {metadata}
        </Text>
      ) : null}
    </Surface>
  );
};

export { StatePanel };
