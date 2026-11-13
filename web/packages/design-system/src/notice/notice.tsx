import { View } from "react-native";

import { Action } from "../interaction";
import { StatusDot, Surface } from "../primitives";
import { Text } from "../text";
import type { Density } from "../tokens";

type NoticeTone = "neutral" | "warn";

type NoticeAction = {
  label: string;
  onPress: () => void;
};

type NoticeProps = {
  /** At most one way out, such as "Try again". */
  action?: NoticeAction;
  /** One plain sentence saying what is true now. */
  body: string;
  density?: Density;
  /** A short sentence-case line saying what happened. */
  title: string;
  /** `warn` outlines the notice in the warning colour; `neutral` is a plain raised card. */
  tone?: NoticeTone;
};

/**
 * A calm line above content that is still on screen: what happened, what is true now, and at most
 * one way out. It stays for as long as the state lasts, so it is a polite status and not an alert
 * that interrupts each time it draws. The dot only tints the title; the words carry the meaning,
 * so the dot is hidden from assistive tech.
 */
const Notice = ({ action, body, density = "pointer", title, tone = "neutral" }: NoticeProps) => (
  <Surface
    aria-live="polite"
    borderColor={tone === "warn" ? "stateWarning" : undefined}
    borderRadius="control"
    gap="control"
    level="raised"
    padding="section"
    role="status"
  >
    <View style={{ alignItems: "center", flexDirection: "row", gap: 8 }}>
      <StatusDot label="" tone={tone === "warn" ? "warn" : "off"} />
      <Text density={density} textRole="headline" tone={tone === "warn" ? "warning" : "primary"}>
        {title}
      </Text>
    </View>
    <Text density={density} textRole="body" tone="secondary">
      {body}
    </Text>
    {action ? (
      <View style={{ alignItems: "flex-start" }}>
        <Action accessibilityRole="button" density={density} onPress={action.onPress} variant="outline">
          {action.label}
        </Action>
      </View>
    ) : null}
  </Surface>
);

export type { NoticeAction, NoticeProps, NoticeTone };
export { Notice };
