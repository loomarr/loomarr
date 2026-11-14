import { View } from "react-native";

import { Action } from "../interaction";
import { StatusDot, Surface } from "../primitives";
import { Text } from "../text";
import type { Density } from "../tokens";

type NoticeTone = "danger" | "neutral" | "success" | "warn";

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
  /**
   * `alert` interrupts assistive tech each time the notice draws, for a failure the person has to
   * hear about now (a `danger` notice after an action they just took). The default `status` is
   * polite.
   */
  role?: "alert" | "status";
  /** A short sentence-case line saying what happened. Without it the body leads. */
  title?: string;
  /**
   * `warn`, `success` and `danger` outline the notice in that state colour; `neutral` is a plain
   * raised card.
   */
  tone?: NoticeTone;
};

const outline = {
  danger: "stateDanger",
  neutral: undefined,
  success: "stateSuccess",
  warn: "stateWarning",
} as const;
const dot = { danger: "error", neutral: "off", success: "ok", warn: "warn" } as const;
const titleTone = { danger: "danger", neutral: "primary", success: "success", warn: "warning" } as const;

/**
 * A calm line above content that is still on screen: what happened, what is true now, and at most
 * one way out. It stays for as long as the state lasts, so by default it is a polite status and
 * not an alert that interrupts each time it draws; `role="alert"` opts into that for a failure. The
 * dot only tints the title; the words carry the meaning, so the dot is hidden from assistive tech.
 */
const Notice = ({
  action,
  body,
  density = "pointer",
  role = "status",
  title,
  tone = "neutral",
}: NoticeProps) => (
  <Surface
    aria-live={role === "alert" ? "assertive" : "polite"}
    borderColor={outline[tone]}
    borderRadius="control"
    gap="control"
    level="raised"
    padding="section"
    role={role}
  >
    <View style={{ alignItems: title ? "center" : "flex-start", flexDirection: "row", gap: 8 }}>
      <View style={title ? undefined : { paddingTop: 6 }}>
        <StatusDot label="" tone={dot[tone]} />
      </View>
      {title ? (
        <Text density={density} textRole="headline" tone={titleTone[tone]}>
          {title}
        </Text>
      ) : (
        <Text density={density} style={{ flexShrink: 1 }} textRole="body" tone="secondary">
          {body}
        </Text>
      )}
    </View>
    {title ? (
      <Text density={density} textRole="body" tone="secondary">
        {body}
      </Text>
    ) : null}
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
