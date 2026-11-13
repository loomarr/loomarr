import { isWeb, useTheme, useThemeName, View } from "@tamagui/core";
import type {
  ComponentRef,
  CSSProperties,
  ForwardRefExoticComponent,
  ReactElement,
  RefAttributes,
} from "react";
import { cloneElement, forwardRef, isValidElement } from "react";
import { type GestureResponderEvent, Pressable } from "react-native";

import { useFocusLook } from "../../focus-look";
import { Icon, type IconTone } from "../../icon";
import { icons } from "../../icons";
import { lineHeightPx } from "../../text";
import { TamaguiText } from "../../text-base";
import {
  brandChroma,
  iconography,
  playerChrome,
  semanticRadius,
  semanticSpace,
  semanticTargets,
  typography,
} from "../../tokens";
import type { ActionProps, ActionVariant } from "./action.type";

const variantLook = (theme: ReturnType<typeof useTheme>, danger: boolean) =>
  ({
    outline: {
      background: "transparent",
      border: danger ? theme.stateDanger.val : theme.borderControl.val,
      iconTone: (danger ? "danger" : "content") as IconTone,
    },
    ghost: { background: "transparent", border: "transparent", iconTone: "content" as IconTone },
    disc: { background: playerChrome.disc, border: playerChrome.disc, iconTone: "inverse" as IconTone },
  }) as const;

const variantTextColor = (theme: ReturnType<typeof useTheme>, variant: ActionVariant, danger: boolean) =>
  variant === "disc"
    ? playerChrome.discInk
    : variant === "outline" && danger
      ? theme.stateDanger.val
      : theme.contentPrimary.val;

// Chains both sides' event handlers and concatenates (never tailwind-merges — this mirrors Base
// UI's own `mergeProps`, which is Tailwind-unaware) `style`/`className` rather than letting
// either side clobber the other, the same composition contract the legacy Button's `render` keeps.
const mergeRenderProps = (
  computed: Record<string, unknown>,
  own: Record<string, unknown> | undefined,
): Record<string, unknown> => {
  const merged: Record<string, unknown> = { ...computed, ...own };
  for (const key of ["onBlur", "onFocus", "onClick"]) {
    const a = computed[key] as ((...args: unknown[]) => void) | undefined;
    const b = own?.[key] as ((...args: unknown[]) => void) | undefined;
    if (a && b)
      merged[key] = (...args: unknown[]) => {
        a(...args);
        b(...args);
      };
  }
  if (computed.style || own?.style)
    merged.style = { ...(computed.style as object | undefined), ...(own?.style as object | undefined) };
  if (computed.className || own?.className)
    merged.className = [computed.className, own?.className].filter(Boolean).join(" ");
  return merged;
};

type ActionRef = ComponentRef<typeof Pressable>;

// Annotated so the exported declaration names Pressable's instance type rather than React Native's
// internal element class.
const Action: ForwardRefExoticComponent<ActionProps & RefAttributes<ActionRef>> = forwardRef<
  ActionRef,
  ActionProps
>(
  (
    {
      accessibilityState,
      "aria-pressed": ariaPressed,
      children,
      density = "pointer",
      disabled = false,
      fitLabel = false,
      icon,
      iconPlacement = "start",
      layout = "inline",
      onBlur,
      onFocus,
      render: renderProp,
      selected = false,
      style,
      submit = false,
      compact = false,
      tabIndex,
      tone = "primary",
      variant,
      ...props
    },
    ref,
  ) => {
    const theme = useTheme();
    const lightTheme = useThemeName().startsWith("light");
    const focusLook = useFocusLook({ onBlur, onFocus });
    const focused = focusLook.focused;
    const render: ReactElement<Record<string, unknown>> | undefined =
      renderProp ??
      (submit && isWeb ? (
        <button
          disabled={disabled === true}
          // A plain style object carries over (a width, an alignSelf); a function style is native's.
          style={typeof style === "object" && style !== null ? (style as CSSProperties) : undefined}
          type="submit"
        />
      ) : undefined);
    // A composed submit button carries its own `disabled` (it must, so Enter in a field cannot
    // submit through it); the Action greys to match rather than drawing an enabled look over it.
    const isDisabled =
      disabled === true || (isWeb && isValidElement(render) && render.props.disabled === true);
    const role = props.accessibilityRole ?? "button";
    const tv = density === "tv";
    const stacked = layout === "stacked" && icon !== undefined;
    const iconOnly = layout === "icon" && icon !== undefined;
    const compactIcon = compact && iconOnly && !tv;
    const textOnly = layout === "text";
    const iconLabel = iconOnly && typeof children === "string" ? children : undefined;
    const look = variant ? variantLook(theme, tone === "danger")[variant] : undefined;
    // A solid fill: an amber focus border cannot show on the amber primary (WCAG 2.4.7), and is
    // under 3:1 on the destructive and suggest fills. A focused fill keeps its rest look
    // and gains a 3px foreground ring outside a gap of the surface behind it, so the ring sits
    // against that surface (about 16:1 in the dark theme) and the gap against the fill (10:1 on
    // amber). The ring is `outline`, which RN 0.86 and react-native-web draw outside the box.
    // A danger tile is not a fill: it is the neutral tile with danger text, as iOS draws a
    // destructive alert button, because a solid red fill reads as on-air, which the system keeps
    // for live.
    const filled = variant ? false : tone === "primary" || tone === "suggest";
    const focusRing =
      filled && focused
        ? {
            outlineColor: theme.contentPrimary.val,
            outlineOffset: tv ? 3 : 2,
            outlineStyle: "solid" as const,
            outlineWidth: 3,
          }
        : undefined;
    // The suggest fill: brandChroma[4] under the dark theme's dark label. The light theme's label is
    // white, 4.1:1 on that magenta, so the light fill is its suggest accent (#A8246F, 6.6:1).
    const suggestFill = lightTheme ? theme.accentSuggest.val : brandChroma[4];
    const backgroundColor = look
      ? look.background
      : tone === "suggest"
        ? suggestFill
        : tone === "primary"
          ? theme.actionPrimary.val
          : selected
            ? theme.surfaceFocus.val
            : theme.surfaceElevated.val;
    // Amber is focus, never selection (#1887). A selected secondary action is neutral-pressed
    // (#1909 Q1/Q2): the `surfaceFocus` fill, a foreground border and a heavier label, so the
    // current item reads from across a room while one amber border still marks where the remote is.
    const pressed = !look && tone === "secondary" && selected;
    const borderColor = look
      ? focused && !filled
        ? theme.actionFocus.val
        : look.border
      : focused && !filled
        ? theme.actionFocus.val
        : tone === "suggest"
          ? suggestFill
          : tone === "primary"
            ? theme.actionPrimary.val
            : pressed
              ? theme.contentPrimary.val
              : stacked
                ? backgroundColor
                : theme.borderControl.val;
    const textColor = variant
      ? variantTextColor(theme, variant, tone === "danger")
      : tone === "danger"
        ? theme.stateDanger.val
        : tone === "success"
          ? theme.stateSuccess.val
          : undefined;
    // A pointer label is already 700, so pressed only lifts the lighter TV label; never thinner.
    const labelWeight = tv ? (pressed ? "600" : "400") : "700";
    const iconTone: IconTone = look
      ? look.iconTone
      : tone === "danger" || tone === "success"
        ? tone
        : tone === "secondary"
          ? selected
            ? "content"
            : "secondary"
          : "inverse";

    const content = textOnly ? (
      <TamaguiText
        color={textColor ?? "contentPrimary"}
        fontFamily="body"
        fontSize={typography[density].hint.size}
        fontWeight={typography[density].hint.weight}
        lineHeight={lineHeightPx(typography[density].hint.lineHeight)}
        textDecorationLine="underline"
      >
        {children}
      </TamaguiText>
    ) : iconOnly && icon ? (
      <Icon
        decorative
        glyph={icons[icon]}
        size={tv ? "touch" : compactIcon ? "compact" : "default"}
        tone={compactIcon && variant === "ghost" ? "muted" : iconTone}
      />
    ) : stacked && icon ? (
      <View alignItems="center" gap={3} paddingVertical={semanticSpace.inline}>
        <Icon
          decorative
          glyph={icons[icon]}
          size="default"
          tone={
            look
              ? look.iconTone
              : tone === "danger" || tone === "success"
                ? tone
                : tone === "secondary"
                  ? "content"
                  : "inverse"
          }
        />
        <TamaguiText
          color={textColor ?? (tone === "secondary" ? "contentSecondary" : "contentInverse")}
          fontFamily="body"
          fontSize={typography[density].cardMeta.size}
          fontWeight={pressed ? "600" : undefined}
          lineHeight={lineHeightPx(typography[density].cardMeta.lineHeight)}
          numberOfLines={1}
        >
          {children}
        </TamaguiText>
      </View>
    ) : icon ? (
      <View
        alignItems="center"
        flexDirection={iconPlacement === "end" ? "row-reverse" : "row"}
        flexShrink={fitLabel ? 1 : undefined}
        gap="inline"
      >
        <Icon decorative glyph={icons[icon]} size={density === "tv" ? "touch" : "default"} tone={iconTone} />
        <TamaguiText
          color={textColor ?? (tone === "secondary" ? "contentPrimary" : "contentInverse")}
          fontFamily="body"
          fontSize={tv ? typography.tv.data.size : typography[density].label.size}
          flexShrink={fitLabel ? 1 : undefined}
          fontWeight={labelWeight}
          {...(fitLabel ? { adjustsFontSizeToFit: true, minimumFontScale: 0.5, numberOfLines: 1 } : {})}
        >
          {children}
        </TamaguiText>
      </View>
    ) : (
      <TamaguiText
        color={textColor ?? (tone === "secondary" ? "contentPrimary" : "contentInverse")}
        fontFamily="body"
        fontSize={tv ? typography.tv.data.size : typography[density].label.size}
        fontWeight={labelWeight}
      >
        {children}
      </TamaguiText>
    );

    const boxStyle = {
      alignItems: "center" as const,
      backgroundColor,
      borderColor,
      borderRadius: tv ? 8 : semanticRadius.control,
      borderStyle: "solid" as const,
      borderWidth: look
        ? borderColor === "transparent"
          ? 0
          : focused
            ? 2
            : 1
        : focused && !filled
          ? tv
            ? 3
            : 4
          : tv && !pressed
            ? 1
            : 2,
      ...focusRing,
      justifyContent: "center" as const,
      minHeight: tv && !iconOnly ? 0 : semanticTargets[density],
      opacity: isDisabled ? 0.55 : 1,
      // A stacked tile shares its row with three others on a phone, so its label gets the width.
      paddingHorizontal: iconOnly ? 0 : tv ? 24 : stacked ? 4 : semanticSpace.control,
      paddingVertical: tv && !iconOnly ? 8 : 0,
      ...(iconOnly ? { width: semanticTargets[density] } : {}),
      ...(compactIcon
        ? {
            borderRadius: semanticSpace.inline / 2,
            height: iconography.size.control,
            minHeight: 0,
            width: iconography.size.control,
          }
        : {}),
      // Text alone: no box to fill or border, and an outline for focus so nothing beside it moves.
      ...(textOnly
        ? {
            backgroundColor: "transparent",
            borderRadius: 2,
            borderWidth: 0,
            minHeight: 0,
            paddingHorizontal: 0,
            paddingVertical: 0,
            ...(focused
              ? {
                  outlineColor: theme.actionFocus.val,
                  outlineOffset: 2,
                  outlineStyle: "solid" as const,
                  outlineWidth: 2,
                }
              : {}),
          }
        : {}),
    };

    if (isWeb && render && isValidElement(render)) {
      // `onPress` is Action's own (Pressable/RN) handler name; a cloned host element only
      // understands `onClick`, so it is bridged here rather than spread through `...props`
      // unchanged, which would silently never fire. `accessibilityRole` and `accessibilityLabel`
      // are RN's too: they reach the host as `role` and `aria-label`, and spread raw they are
      // unknown DOM attributes that leave the element unnamed.
      // A composing parent's `onClick` (Base UI's DialogTrigger, which hands its props to the
      // Action it renders) is chained with `onPress`, not overwritten by it.
      const {
        accessibilityLabel,
        accessibilityRole: _role,
        onClick: parentClick,
        onPress,
        ...restProps
      } = props as typeof props & {
        onClick?: (event: unknown) => void;
        onPress?: (event: unknown) => void;
      };
      // The host's `style` is CSS, which has no paddingHorizontal/paddingVertical: spell them out.
      const { paddingHorizontal, paddingVertical, ...hostStyle } = boxStyle;
      return cloneElement(
        render,
        mergeRenderProps(
          {
            "aria-label": accessibilityLabel ?? iconLabel,
            ...restProps,
            "aria-disabled": isDisabled || undefined,
            "aria-pressed": ariaPressed,
            onBlur: focusLook.onBlur,
            onClick:
              parentClick && onPress
                ? (event: unknown) => {
                    parentClick(event);
                    onPress(event);
                  }
                : (parentClick ?? onPress),
            onFocus: focusLook.onFocus,
            ref,
            role,
            style: {
              display: "inline-flex",
              ...hostStyle,
              paddingBottom: paddingVertical,
              paddingLeft: paddingHorizontal,
              paddingRight: paddingHorizontal,
              paddingTop: paddingVertical,
            },
            tabIndex: isDisabled ? -1 : tabIndex,
          },
          render.props,
        ),
        content,
      ) as ReactElement;
    }

    // react-native-web's Pressable answers clicks through its own press responder and never calls
    // an `onClick` prop, so a composing parent's (see the render branch) runs from onPress here.
    const {
      onClick: parentClick,
      onPress,
      ...pressableProps
    } = props as typeof props & { onClick?: (event: GestureResponderEvent) => void };
    const press =
      parentClick && isWeb
        ? (event: GestureResponderEvent) => {
            parentClick(event);
            onPress?.(event);
          }
        : onPress;

    return (
      <Pressable
        accessibilityLabel={iconLabel}
        {...pressableProps}
        onPress={press}
        accessibilityState={{ ...accessibilityState, disabled: isDisabled, selected }}
        accessibilityRole={role}
        aria-disabled={isDisabled || undefined}
        aria-pressed={role === "button" && selected ? true : ariaPressed}
        aria-selected={role === "tab" ? selected : undefined}
        disabled={isDisabled}
        onBlur={focusLook.onBlur}
        onFocus={focusLook.onFocus}
        ref={ref}
        tabIndex={isDisabled ? -1 : tabIndex}
        style={(state) => [
          {
            ...boxStyle,
            // A press dims; nothing scales (#2789: hover, focus and press are colour and border).
            opacity: isDisabled ? 0.55 : state.pressed ? 0.82 : 1,
          },
          typeof style === "function" ? style(state) : style,
        ]}
      >
        {content}
      </Pressable>
    );
  },
);

Action.displayName = "Action";

export { Action };
