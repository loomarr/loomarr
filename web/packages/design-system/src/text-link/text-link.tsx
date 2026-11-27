import { isWeb, useTheme } from "@tamagui/core";
import type { CSSProperties, ReactElement, ReactNode } from "react";
import { cloneElement, createElement, isValidElement, useState } from "react";

import { useFocusLook } from "../focus-look";
import { Icon } from "../icon";
import { type IconName, icons } from "../icons";
import { TamaguiText } from "../text-base";

/**
 * Which colour the link rests in. `info` (the default) is the cyan that marks a link. The others are
 * for a link whose surroundings already say what colour it is, so a page can move onto the design
 * system without recolouring its links: `inherit` takes the text around it, `primary` the amber of
 * the primary action, `muted` the quiet secondary text colour, which brightens to the primary text
 * colour under the pointer (a quiet link that wakes up when you reach for it).
 */
type TextLinkTone = "info" | "inherit" | "muted" | "primary";

// The hover state of a web link, chained to the element's own handlers like `useFocusLook` does for
// focus. The pointer is the only thing that sets it, so a touch tap never leaves a link "hovered".
const useHoverLook = (own: {
  onMouseEnter?: (event: unknown) => void;
  onMouseLeave?: (event: unknown) => void;
}) => {
  const [hovered, setHovered] = useState(false);
  return {
    hovered,
    onMouseEnter: (event: unknown) => {
      setHovered(true);
      own.onMouseEnter?.(event);
    },
    onMouseLeave: (event: unknown) => {
      setHovered(false);
      own.onMouseLeave?.(event);
    },
  };
};

type TextLinkProps = {
  /** The label, sentence case: "Open on Archive.org". It names the link, so say where it goes. */
  children: ReactNode;
  "aria-label"?: string;
  /**
   * Opens somewhere that is not Loomarr, in a new tab, without sending a referrer. It also draws the
   * "leaves Loomarr" glyph after the label unless `icon` names another.
   */
  external?: boolean;
  /** Web: the address. With `render`, leave it to the element. */
  href?: string;
  /** A glyph beside the label, decorative: the name is the label alone. */
  icon?: IconName;
  /** `end` (the default) points the way the link goes; `start` leads the label, as a "Back" does. */
  iconPlacement?: "end" | "start";
  /**
   * Native: the press. On the web a link with an address or a `render` element follows that; a link
   * with a press and no address is a button drawn as a link, for a reference the page routes itself.
   */
  onPress?: () => void;
  /**
   * Web: draws the link's look onto this element in place of an `<a>`, so an in-app link is the
   * router's own `Link` and navigates without a page load: `render={<Link to="/help" />}`. Its own
   * props are kept and its children are replaced by the label.
   */
  render?: ReactElement<Record<string, unknown>>;
  /** The colour it rests in, `info` (the default) unless the surrounding text already sets one. */
  tone?: TextLinkTone;
  /**
   * `always` (the default) underlines the link so it is not told apart by colour alone. `hover` draws
   * the underline only under the pointer, for a link whose position (a title in a list, a link
   * beside its own icon) already says it is one. Native has no pointer, so there it stays
   * underlined. A `muted` link that is always underlined draws the line in the quiet border colour,
   * so the line reads as a hairline under quiet text.
   */
  underline?: "always" | "hover";
};

// Native has no theme-inheriting `color` prop value for `inherit`: leaving it unset takes the text
// around it.
const nativeColour = {
  info: "stateInfo",
  inherit: undefined,
  muted: "contentMuted",
  primary: "actionPrimary",
} as const;

const webColour = (tone: TextLinkTone, hovered: boolean, theme: ReturnType<typeof useTheme>) => {
  if (tone === "inherit") return "inherit";
  if (tone === "primary") return theme.actionPrimary.val;
  if (tone === "muted") return hovered ? theme.contentPrimary.val : theme.contentMuted.val;
  return theme.stateInfo.val;
};

/**
 * A link written into the page: "Open on Archive.org" with its way-out glyph, a "Fix" that goes to
 * the Help center, a source cited in a line of prose. It takes the surrounding text's own type, the
 * info colour that marks a link (the cyan "Open on Archive.org" already draws), an underline so it is
 * not told apart by colour alone, and the one amber outline for focus (§9). A button that does
 * something where you are is `InlineAction`; a link that is a box of its own is `Action` with
 * `accessibilityRole="link"`.
 */
const TextLink = ({
  "aria-label": ariaLabel,
  children,
  external = false,
  href,
  icon,
  iconPlacement = "end",
  onPress,
  render,
  tone = "info",
  underline = "always",
}: TextLinkProps) => {
  const theme = useTheme();
  const renderProps = (isValidElement(render) ? render.props : {}) as {
    onBlur?: (event: unknown) => void;
    onFocus?: (event: unknown) => void;
    onMouseEnter?: (event: unknown) => void;
    onMouseLeave?: (event: unknown) => void;
  };
  const focus = useFocusLook<unknown>(renderProps);
  const hover = useHoverLook(renderProps);
  const glyph = icon ?? (external ? "external" : undefined);

  if (!isWeb) {
    return (
      <TamaguiText
        accessibilityLabel={ariaLabel}
        accessibilityRole="link"
        color={nativeColour[tone]}
        onPress={onPress}
        textDecorationLine="underline"
      >
        {children}
      </TamaguiText>
    );
  }

  const icon$ = glyph ? <Icon decorative glyph={icons[glyph]} size="compact" tone="inherit" /> : null;
  const content = (
    <>
      {iconPlacement === "start" && icon$}
      {children}
      {iconPlacement === "end" && icon$}
    </>
  );

  const style: CSSProperties = {
    borderRadius: 2,
    color: webColour(tone, hover.hovered, theme),
    cursor: "pointer",
    // Inline flow, so a link in a sentence wraps with its words; a glyph needs a flex row to sit
    // beside the label on one line.
    display: glyph ? "inline-flex" : "inline",
    font: "inherit",
    letterSpacing: "inherit",
    textDecorationLine: underline === "always" || hover.hovered ? "underline" : "none",
    textUnderlineOffset: 2,
    ...(tone === "muted" && underline === "always"
      ? { textDecorationColor: theme.borderDecorative.val }
      : {}),
    ...(glyph ? { alignItems: "center", gap: 4 } : {}),
    ...(focus.focused
      ? { outline: `2px solid ${theme.actionFocus.val}`, outlineOffset: 2 }
      : { outline: "none" }),
  };

  const own = {
    "aria-label": ariaLabel,
    "data-touch-target": true,
    onBlur: focus.onBlur,
    onFocus: focus.onFocus,
    onMouseEnter: hover.onMouseEnter,
    onMouseLeave: hover.onMouseLeave,
    ...(external ? { rel: "noreferrer", target: "_blank" } : {}),
  };

  if (render && isValidElement(render)) {
    // The element's own props win, except the focus and hover handlers (chained by the hooks) and
    // `style`.
    return cloneElement(
      render,
      {
        ...own,
        ...render.props,
        onBlur: focus.onBlur,
        onFocus: focus.onFocus,
        onMouseEnter: hover.onMouseEnter,
        onMouseLeave: hover.onMouseLeave,
        style: { ...style, ...(render.props.style as CSSProperties | undefined) },
      },
      content,
    );
  }

  // With no address, a press is the whole link: a cross-page reference inside rendered documentation
  // that routes through the page's own handler. It stays a button (Enter, Space, a focus stop) with
  // the browser's button chrome reset, drawn as the same link.
  if (!href && onPress) {
    return createElement(
      "button",
      {
        ...own,
        onClick: () => onPress(),
        style: { ...style, background: "none", border: 0, margin: 0, padding: 0 },
        type: "button",
      },
      content,
    );
  }

  return createElement("a", { ...own, href, onClick: onPress ? () => onPress() : undefined, style }, content);
};

export type { TextLinkProps, TextLinkTone };
export { TextLink };
