import { isWeb, useTheme } from "@tamagui/core";
import type { CSSProperties, ReactElement, ReactNode } from "react";
import { cloneElement, createElement, isValidElement } from "react";

import { useFocusLook } from "../focus-look";
import { Icon } from "../icon";
import { type IconName, icons } from "../icons";
import { TamaguiText } from "../text-base";

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
}: TextLinkProps) => {
  const theme = useTheme();
  const focus = useFocusLook<unknown>(
    (isValidElement(render) ? render.props : {}) as {
      onBlur?: (event: unknown) => void;
      onFocus?: (event: unknown) => void;
    },
  );
  const glyph = icon ?? (external ? "external" : undefined);

  if (!isWeb) {
    return (
      <TamaguiText
        accessibilityLabel={ariaLabel}
        accessibilityRole="link"
        color="stateInfo"
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
    color: theme.stateInfo.val,
    cursor: "pointer",
    // Inline flow, so a link in a sentence wraps with its words; a glyph needs a flex row to sit
    // beside the label on one line.
    display: glyph ? "inline-flex" : "inline",
    font: "inherit",
    letterSpacing: "inherit",
    textDecorationLine: "underline",
    textUnderlineOffset: 2,
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
    ...(external ? { rel: "noreferrer", target: "_blank" } : {}),
  };

  if (render && isValidElement(render)) {
    // The element's own props win, except the focus handlers (chained by the hook) and `style`.
    return cloneElement(
      render,
      {
        ...own,
        ...render.props,
        onBlur: focus.onBlur,
        onFocus: focus.onFocus,
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

export type { TextLinkProps };
export { TextLink };
