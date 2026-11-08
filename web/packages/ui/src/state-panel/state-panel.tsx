import { Action, ActivityIndicator, BrandMark, Surface, Text } from "@loomarr/design-system";

import type { StatePanelProps } from "./state-panel.type";

// One family for every empty list, error, 404, cover and onboarding: the static colour bars (4px), a
// sentence-case heading, one plain sentence and at most one action. The bars are the test card a set
// showed when nothing was on, so they stay still; only loading swaps them for a spinner.
const StatePanel = ({ action, description, density = "pointer", icon, kind, title }: StatePanelProps) => {
  const loading = kind === "loading";

  return (
    <Surface
      aria-busy={loading || undefined}
      aria-live={kind === "error" ? "assertive" : "polite"}
      alignItems="center"
      gap="control"
      justifyContent="center"
      minHeight={density === "tv" ? 320 : 220}
      padding={density === "tv" ? 48 : "section"}
      width="100%"
      level="raised"
      role={kind === "error" ? "alert" : "status"}
    >
      {icon ??
        (loading ? (
          <Surface alignItems="center" backgroundColor="transparent" borderWidth={0}>
            <ActivityIndicator accessibilityLabel={title} size={density === "tv" ? "tv" : "control"} />
          </Surface>
        ) : (
          <BrandMark contained={false} decorative size={4} />
        ))}
      <Surface
        alignItems="center"
        backgroundColor="transparent"
        borderWidth={0}
        gap="inline"
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
    </Surface>
  );
};

export { StatePanel };
