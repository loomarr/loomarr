import { isWeb, useTheme, View } from "@tamagui/core";
import type { ComponentProps, ComponentRef, ReactNode } from "react";
import { forwardRef, useId, useLayoutEffect, useRef, useState } from "react";
import { Pressable, ScrollView, useWindowDimensions } from "react-native";

import { useFocusLook } from "../focus-look";
import { StatusDot, type StatusTone, Text } from "../primitives";
import { type Density, semanticSpace, semanticTargets } from "../tokens";
import { useViewportScale } from "../viewport";

type AdaptiveDirection = "column" | "row";

// The window's width in layout units: points, except on Apple TV, where the viewport stage lays
// the app out at TV width. A component outside the design system reads it here, not from
// react-native, so Storybook's docgen never has to parse the native package's Flow source.
const useWindowWidth = () => useWindowDimensions().width / useViewportScale();

const adaptiveBreakpoints: Record<Density, number> = {
  pointer: 900,
  touch: 720,
  tv: 1280,
};

const resolveAdaptiveDirection = (
  width: number,
  density: Density,
  breakpoint = adaptiveBreakpoints[density],
): AdaptiveDirection => (width >= breakpoint ? "row" : "column");

type AdaptiveSplitProps = Omit<ComponentProps<typeof View>, "children" | "flexDirection"> & {
  breakpoint?: number;
  density?: Density;
  primary: ReactNode;
  secondary: ReactNode;
  secondaryWidth?: number | string;
};

/**
 * A two-region layout that owns the wide/narrow transition. Product modules supply regions,
 * not viewport checks, so web and native hosts share the same composition rule.
 */
const AdaptiveSplit = ({
  breakpoint,
  density = "pointer",
  gap = "section",
  primary,
  secondary,
  secondaryWidth,
  ...props
}: AdaptiveSplitProps) => {
  const width = useWindowWidth();
  const direction = resolveAdaptiveDirection(width, density, breakpoint);
  const isWide = direction === "row";
  const resolvedSecondaryWidth = secondaryWidth ?? (density === "tv" ? 520 : 360);

  return (
    <View {...props} alignItems="stretch" flexDirection={direction} gap={gap} width="100%">
      <View flex={isWide ? 1 : undefined} minWidth={0} width={isWide ? undefined : "100%"}>
        {primary}
      </View>
      <View flexShrink={0} width={isWide ? resolvedSecondaryWidth : "100%"}>
        {secondary}
      </View>
    </View>
  );
};

type ScrollFrameProps = Omit<
  ComponentProps<typeof ScrollView>,
  "children" | "contentContainerStyle" | "horizontal"
> & {
  children: ReactNode;
  contentContainerStyle?: ComponentProps<typeof ScrollView>["contentContainerStyle"];
  density?: Density;
};

/** A vertical application scroller with consistent content rhythm and keyboard behavior. */
const ScrollFrame = forwardRef<ComponentRef<typeof ScrollView>, ScrollFrameProps>(
  (
    {
      children,
      contentContainerStyle,
      density = "pointer",
      keyboardShouldPersistTaps = "handled",
      style,
      ...props
    },
    ref,
  ) => (
    <ScrollView
      {...props}
      contentContainerStyle={[
        { flexGrow: 1, gap: density === "tv" ? semanticSpace.section : semanticSpace.control },
        contentContainerStyle,
      ]}
      keyboardShouldPersistTaps={keyboardShouldPersistTaps}
      ref={ref}
      style={[{ flex: 1 }, style]}
    >
      {children}
    </ScrollView>
  ),
);

ScrollFrame.displayName = "ScrollFrame";

type DisclosureProps = Omit<ComponentProps<typeof View>, "children"> & {
  children: ReactNode;
  /** The starting state when `expanded` is not given (uncontrolled). */
  defaultExpanded?: boolean;
  density?: Density;
  description?: string;
  disabled?: boolean;
  /** Controlled state; leave it out and the disclosure keeps its own, from `defaultExpanded`. */
  expanded?: boolean;
  label: string;
  onExpandedChange?: (expanded: boolean) => void;
  /**
   * A status dot before the label (a connection's ok / failing / untested). Colour alone says
   * nothing, so put the words in `description` ("OK", "needs attention").
   */
  status?: StatusTone;
  /** What the status dot means, for assistive tech. Defaults to "" (the description says it). */
  statusLabel?: string;
};

// The collapsed body on web: still mounted (its state survives), but `hidden="until-found"`, so it
// is out of the accessibility tree and the tab order while the browser's find-in-page can still
// reach its text. A match fires `beforematch` just before the browser reveals it, and that opens
// the disclosure for real. React renders `hidden` as a boolean, so the attribute is set here.
// Typed by what is used, not as HTMLDivElement: packages that compile this file without the DOM lib
// (the shared UI package) would otherwise see an element with none of these members.
type FindableNode = {
  addEventListener: (type: string, listener: () => void) => void;
  removeAttribute: (name: string) => void;
  removeEventListener: (type: string, listener: () => void) => void;
  setAttribute: (name: string, value: string) => void;
};

const useFindableBody = (expanded: boolean, onFound: () => void) => {
  const body = useRef<FindableNode | null>(null);
  const found = useRef(onFound);
  found.current = onFound;

  useLayoutEffect(() => {
    const node = body.current;
    if (!node) return undefined;
    if (expanded) {
      node.removeAttribute("hidden");
      return undefined;
    }
    node.setAttribute("hidden", "until-found");
    const reveal = () => found.current();
    node.addEventListener("beforematch", reveal);
    return () => node.removeEventListener("beforematch", reveal);
  }, [expanded]);

  return body;
};

/**
 * Shared progressive disclosure for optional detail. It owns expanded semantics, focus treatment,
 * and the target size while allowing each host to use its native Pressable mechanics. Controlled
 * through `expanded` + `onExpandedChange`, or uncontrolled from `defaultExpanded`.
 *
 * On web a collapsed body stays mounted and findable in-page (see `useFindableBody`); on native it
 * unmounts, as there is no find-in-page to serve.
 */
const Disclosure = ({
  children,
  defaultExpanded = false,
  density = "pointer",
  description,
  disabled = false,
  expanded: controlled,
  label,
  onExpandedChange,
  status,
  statusLabel = "",
  ...props
}: DisclosureProps) => {
  const generatedId = useId();
  const contentId = `loomarr-disclosure-${generatedId.replaceAll(":", "")}`;
  const { focused, ...focusHandlers } = useFocusLook();
  const theme = useTheme();
  const [own, setOwn] = useState(defaultExpanded);
  const expanded = controlled ?? own;
  const change = (next: boolean) => {
    if (controlled === undefined) setOwn(next);
    onExpandedChange?.(next);
  };
  const findable = useFindableBody(expanded, () => change(true));

  const body = (
    <View
      borderTopColor="borderDecorative"
      borderTopWidth={1}
      gap="control"
      padding="control"
      // `id` alone: Tamagui maps it on native, and since Tamagui 3 a `nativeID` reaches the web
      // DOM as an unknown attribute, which React warns about. On web the findable wrapper carries
      // it instead, so `aria-controls` names an element that exists while collapsed too.
      id={isWeb ? undefined : contentId}
    >
      {children}
    </View>
  );

  return (
    <View
      {...props}
      backgroundColor="surfaceRaised"
      borderColor={focused ? "actionFocus" : "borderDecorative"}
      borderRadius="card"
      borderWidth={focused ? 3 : 1}
      overflow="hidden"
      width="100%"
    >
      <Pressable
        accessibilityRole="button"
        accessibilityState={{ disabled, expanded }}
        aria-controls={contentId}
        aria-disabled={disabled || undefined}
        aria-expanded={expanded}
        disabled={disabled}
        {...focusHandlers}
        onPress={() => change(!expanded)}
        style={({ pressed }) => ({
          alignItems: "center",
          backgroundColor: focused ? theme.surfaceFocus.val : theme.surfaceRaised.val,
          flexDirection: "row",
          gap: semanticSpace.control,
          minHeight: semanticTargets[density],
          opacity: disabled ? 0.55 : pressed ? 0.82 : 1,
          paddingHorizontal: semanticSpace.control,
          paddingVertical: semanticSpace.inline,
        })}
      >
        {status ? <StatusDot label={statusLabel} tone={status} /> : null}
        <View alignItems="flex-start" flex={1} gap={2}>
          <Text density={density} textAlign="left" textRole="label" tone="primary">
            {label}
          </Text>
          {/* Hint, not metadata: the same size at every density, in the body face. A description
              and Show/Hide are words, and mono is for channel numbers and clocks (#2778, #3412). */}
          {description ? (
            <Text density={density} textAlign="left" textRole="hint" tone="secondary">
              {description}
            </Text>
          ) : null}
        </View>
        <Text density={density} textRole="hint" tone={focused ? "primary" : "secondary"}>
          {expanded ? "Hide" : "Show"}
        </Text>
      </Pressable>
      {isWeb ? (
        <div
          id={contentId}
          ref={(node) => {
            findable.current = node as unknown as FindableNode | null;
          }}
        >
          {body}
        </div>
      ) : expanded ? (
        body
      ) : null}
    </View>
  );
};

type SectionHeaderProps = {
  /** Trailing controls at the end of the row: an `Action variant="link"`, or a small cluster. */
  children?: ReactNode;
  density?: Density;
  /** The heading's id, so the section can point `aria-labelledby` at it. */
  id?: string;
  /** The quiet count or explanation beside the title ("3 people", "14 titles · 1 new channel"). */
  meta?: ReactNode;
  title: string;
};

/**
 * A section's title, its meta, and trailing controls (#1659 web mock: Home's Watching now, On now,
 * Tonight). The row wraps: at a narrow width the controls drop to their own line instead of
 * squeezing the title into a word-per-line column.
 */
const SectionHeader = ({ children, density = "pointer", id, meta, title }: SectionHeaderProps) => (
  <View
    alignItems="center"
    columnGap={semanticSpace.control}
    flexDirection="row"
    flexWrap="wrap"
    marginBottom={semanticSpace.control}
    rowGap={4}
  >
    <Text
      density={density}
      id={id}
      strong
      textRole="headline"
      {...(isWeb ? { as: "h2", style: { margin: 0 } } : { accessibilityRole: "header" as const })}
    >
      {title}
    </Text>
    {meta != null ? (
      <Text density={density} textRole="caption" tone="secondary">
        {meta}
      </Text>
    ) : null}
    {children != null ? (
      <View alignItems="center" flexDirection="row" gap={semanticSpace.control} marginLeft="auto">
        {children}
      </View>
    ) : null}
  </View>
);

export type { AdaptiveDirection, AdaptiveSplitProps, DisclosureProps, ScrollFrameProps, SectionHeaderProps };
export {
  AdaptiveSplit,
  adaptiveBreakpoints,
  Disclosure,
  resolveAdaptiveDirection,
  ScrollFrame,
  SectionHeader,
  useWindowWidth,
};
