import { isWeb, useTheme, View } from "@tamagui/core";
import type {
  ComponentProps,
  ComponentRef,
  ForwardRefExoticComponent,
  ReactElement,
  ReactNode,
  RefAttributes,
} from "react";
import { cloneElement, forwardRef, isValidElement, useState } from "react";
import { Platform, Pressable, TextInput } from "react-native";

import { Icon, type IconTone } from "../icon";
import { type IconName, icons } from "../icons";
import { useBehindModal } from "../modal-layer";
import { lineHeightPx, Surface, Text } from "../primitives";
import { TamaguiText } from "../text-base";
import {
  brandChroma,
  type Density,
  nativeFontFace,
  semanticRadius,
  semanticSpace,
  semanticTargets,
  typography,
} from "../tokens";

/**
 * The legacy Web Button's six flat variants (#970 PR B checkpoint 3), alongside `tone`'s existing
 * filled/bordered tile look. Deliberately a separate prop rather than new `tone` values: `tone`
 * always draws a border and a focus ring sized for remote/touch targets (TV and phone callers
 * depend on that), while these are flat, underline-on-hover-link-included, and match the legacy
 * Button's own flatter aesthetic. Supplying both prefers `variant`.
 */
type ActionVariant = "destructive" | "ghost" | "link" | "outline" | "secondary" | "suggest";

type ActionProps = Omit<ComponentProps<typeof Pressable>, "children" | "style"> & {
  children: ReactNode;
  density?: Density;
  icon?: IconName;
  /**
   * Keep an inline label on one line by shrinking it to the room beside the icon, rather than
   * breaking a word that cannot fit (a 2×2 tile at an accessibility text size).
   */
  fitLabel?: boolean;
  /**
   * `stacked` puts the icon over a small label, on a tile with no outline until focused: the row of
   * player controls under a phone's picture (#1659 native mock 5e). Give it an icon.
   */
  layout?: "inline" | "stacked";
  /**
   * Web-only composition escape hatch (Base UI's `render` prop equivalent, #970 PR B checkpoint 3):
   * renders Action's resolved style and content onto this element instead of a `Pressable`, so a
   * button-like control can compose onto a router `Link`. The element's own children are replaced
   * by Action's `children`; its own event handlers, `style` and `className` are preserved and
   * merged with Action's. Ignored on native, where there is no comparable host-element swap.
   *
   * Composing this way loses the `Pressable` press-scale micro-interaction (there is no gesture
   * responder to drive it), and `layout="stacked"`/`selected` are not meaningful on a transparent,
   * un-boxed renderer — this is for a plain `variant` button standing in for a link, not a tile.
   */
  render?: ReactElement<Record<string, unknown>>;
  selected?: boolean;
  style?: ComponentProps<typeof Pressable>["style"];
  tone?: "danger" | "primary" | "secondary";
  variant?: ActionVariant;
};

const variantLook = (theme: ReturnType<typeof useTheme>) =>
  ({
    // Dark text on a solid accent: suggest/destructive both fail AA with light text (§2.1,
    // carried over from the legacy cva calibration), so `inverse` (dark) is deliberate, not a
    // mistake inherited from `tone`'s own inverse-on-filled convention.
    suggest: { background: brandChroma[4], border: "transparent", iconTone: "inverse" as IconTone },
    destructive: { background: theme.guideOnAir.val, border: "transparent", iconTone: "inverse" as IconTone },
    outline: { background: "transparent", border: theme.borderControl.val, iconTone: "content" as IconTone },
    secondary: {
      background: theme.surfaceElevated.val,
      border: "transparent",
      iconTone: "content" as IconTone,
    },
    ghost: { background: "transparent", border: "transparent", iconTone: "content" as IconTone },
    link: { background: "transparent", border: "transparent", iconTone: "info" as IconTone },
  }) as const;

const variantTextColor = (theme: ReturnType<typeof useTheme>, variant: ActionVariant) =>
  variant === "suggest" || variant === "destructive"
    ? theme.contentInverse.val
    : variant === "link"
      ? theme.stateInfo.val
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
      children,
      density = "pointer",
      disabled = false,
      fitLabel = false,
      icon,
      layout = "inline",
      onBlur,
      onFocus,
      render,
      selected = false,
      style,
      tabIndex,
      tone = "primary",
      variant,
      ...props
    },
    ref,
  ) => {
    const theme = useTheme();
    const [hasFocus, setFocused] = useState(false);
    // A native Modal never blurs its opener, so a control behind an open modal drops its focus look.
    const behindModal = useBehindModal();
    const focused = hasFocus && !behindModal;
    const isDisabled = disabled === true;
    const role = props.accessibilityRole ?? "button";
    const tv = density === "tv";
    const stacked = layout === "stacked" && icon !== undefined;
    const look = variant ? variantLook(theme)[variant] : undefined;
    // A solid fill: an amber focus border cannot show on the amber primary (WCAG 2.4.7), and is
    // under 3:1 on the destructive and suggest fills. A focused fill keeps its rest look
    // and gains a 3px foreground ring outside a gap of the surface behind it, so the ring sits
    // against that surface (about 16:1 in the dark theme) and the gap against the fill (10:1 on
    // amber). The ring is `outline`, which RN 0.86 and react-native-web draw outside the box.
    // A danger tile is not a fill: it is the neutral tile with danger text, as iOS draws a
    // destructive alert button, because a solid red fill reads as on-air, which the system keeps
    // for live.
    const filled = variant ? variant === "suggest" || variant === "destructive" : tone === "primary";
    const focusRing =
      filled && focused
        ? {
            outlineColor: theme.contentPrimary.val,
            outlineOffset: tv ? 3 : 2,
            outlineStyle: "solid" as const,
            outlineWidth: 3,
          }
        : undefined;
    const backgroundColor = look
      ? look.background
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
        : tone === "primary"
          ? theme.actionPrimary.val
          : pressed
            ? theme.contentPrimary.val
            : stacked
              ? backgroundColor
              : theme.borderControl.val;
    const textColor = variant
      ? variantTextColor(theme, variant)
      : tone === "danger"
        ? theme.stateDanger.val
        : undefined;
    // A pointer label is already 700, so pressed only lifts the lighter TV label; never thinner.
    const labelWeight = tv ? (pressed ? "600" : "400") : "700";
    const iconTone: IconTone = look
      ? look.iconTone
      : tone === "danger"
        ? "danger"
        : tone === "secondary"
          ? selected
            ? "content"
            : "secondary"
          : "inverse";

    const content =
      stacked && icon ? (
        <View alignItems="center" gap={3} paddingVertical={semanticSpace.inline}>
          <Icon
            decorative
            glyph={icons[icon]}
            size="default"
            tone={
              look
                ? look.iconTone
                : tone === "danger"
                  ? "danger"
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
        <View alignItems="center" flexDirection="row" flexShrink={fitLabel ? 1 : undefined} gap="inline">
          <Icon
            decorative
            glyph={icons[icon]}
            size={density === "tv" ? "touch" : "default"}
            tone={iconTone}
          />
          <TamaguiText
            color={textColor ?? (tone === "secondary" ? "contentPrimary" : "contentInverse")}
            fontFamily="body"
            fontSize={tv ? typography.tv.data.size : typography[density].label.size}
            flexShrink={fitLabel ? 1 : undefined}
            fontWeight={labelWeight}
            textDecorationLine={variant === "link" ? "underline" : undefined}
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
          textDecorationLine={variant === "link" ? "underline" : undefined}
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
      minHeight: tv ? 0 : semanticTargets[density],
      opacity: isDisabled ? 0.55 : 1,
      // A stacked tile shares its row with three others on a phone, so its label gets the width.
      paddingHorizontal: tv ? 24 : stacked ? 4 : semanticSpace.control,
      paddingVertical: tv ? 8 : 0,
    };

    if (isWeb && render && isValidElement(render)) {
      // `onPress` is Action's own (Pressable/RN) handler name; a cloned host element only
      // understands `onClick`, so it is bridged here rather than spread through `...props`
      // unchanged, which would silently never fire.
      const { onPress, ...restProps } = props as typeof props & {
        onPress?: (event: unknown) => void;
      };
      return cloneElement(
        render,
        mergeRenderProps(
          {
            ...restProps,
            "aria-disabled": isDisabled || undefined,
            onBlur: () => setFocused(false),
            onClick: onPress,
            onFocus: () => setFocused(true),
            ref,
            role,
            style: { display: "inline-flex", ...boxStyle },
            tabIndex: isDisabled ? -1 : tabIndex,
          },
          render.props,
        ),
        content,
      ) as ReactElement;
    }

    return (
      <Pressable
        {...props}
        accessibilityState={{ ...accessibilityState, disabled: isDisabled, selected }}
        accessibilityRole={role}
        aria-disabled={isDisabled || undefined}
        aria-pressed={role === "button" && selected ? true : undefined}
        aria-selected={role === "tab" ? selected : undefined}
        disabled={isDisabled}
        onBlur={(event) => {
          setFocused(false);
          onBlur?.(event);
        }}
        onFocus={(event) => {
          setFocused(true);
          onFocus?.(event);
        }}
        ref={ref}
        tabIndex={isDisabled ? -1 : tabIndex}
        style={(state) => [
          {
            ...boxStyle,
            opacity: isDisabled ? 0.55 : state.pressed ? 0.82 : 1,
            transform: [
              { scale: tv ? (state.pressed ? 0.98 : 1) : focused ? 1.025 : state.pressed ? 0.98 : 1 },
            ],
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

type FieldProps = Omit<ComponentProps<typeof TextInput>, "editable"> & {
  density?: Density;
  description?: string;
  disabled?: boolean;
  error?: string;
  invalid?: boolean;
  label?: string;
};

const Field = ({
  accessibilityLabel,
  density = "pointer",
  description,
  disabled = false,
  error,
  invalid = false,
  label,
  onBlur,
  onFocus,
  style,
  ...props
}: FieldProps) => {
  const theme = useTheme();
  const [focused, setFocused] = useState(false);
  const hasError = invalid || Boolean(error);
  const borderColor = hasError
    ? theme.stateDanger.val
    : focused
      ? theme.actionFocus.val
      : theme.borderControl.val;

  return (
    <View gap="inline" width="100%">
      {label ? (
        <Text density={density} textRole="label">
          {label}
        </Text>
      ) : null}
      <TextInput
        {...props}
        accessibilityLabel={accessibilityLabel ?? label}
        accessibilityState={{ disabled }}
        aria-disabled={disabled || undefined}
        aria-invalid={hasError || undefined}
        editable={!disabled}
        onBlur={(event) => {
          setFocused(false);
          onBlur?.(event);
        }}
        onFocus={(event) => {
          setFocused(true);
          onFocus?.(event);
        }}
        placeholderTextColor={theme.contentMuted.val}
        style={[
          {
            backgroundColor: disabled ? theme.surfaceRaised.val : theme.surfaceCanvas.val,
            borderColor,
            borderRadius: semanticRadius.control,
            borderWidth: focused ? 3 : 2,
            color: theme.contentPrimary.val,
            // A raw TextInput: Tamagui's per-weight face swap doesn't reach it, so name the file.
            fontFamily: isWeb
              ? typography.family.data.web
              : nativeFontFace("data", Platform.OS === "android" ? "android" : "ios"),
            fontSize: typography[density].body.size,
            minHeight: semanticTargets[density],
            opacity: disabled ? 0.62 : 1,
            paddingHorizontal: semanticSpace.control,
          },
          style,
        ]}
      />
      {error ? (
        <Text aria-live="polite" density={density} textRole="metadata" tone="danger">
          {error}
        </Text>
      ) : description ? (
        <Text density={density} textRole="metadata">
          {description}
        </Text>
      ) : null}
    </View>
  );
};

type FieldHintProps = {
  children: ReactNode;
  /** Web layout hooks only (a grid span, a gap beside an icon). Never colour or type: those are the hint's own. */
  className?: string;
  density?: Density;
  /** The id a control's `aria-describedby` points at, so the hint is read with the field. */
  id?: string;
  /**
   * `alert` announces a validation message the moment it appears. Leave it unset for standing help,
   * which a screen reader reaches through the control's `aria-describedby` instead.
   */
  role?: "alert" | "status";
  tone?: "error" | "help";
};

/**
 * A form field's help or validation line (#970 step 45). On the Web it is a real `<p>`, so a native
 * `<input>`'s `aria-describedby` resolves to it and its text joins the control's accessible
 * description; the control itself stays a browser element, keeping form submission and the
 * browser's own validation. Help reads in the secondary content colour, an error in the danger
 * colour: the two Web conventions it replaces (`text-muted-foreground` / `text-onair-300`).
 *
 * Field's own description and error lines keep their `metadata` measure: the native field has
 * its own approved look, and folding it onto this one would change it.
 */
const FieldHint = ({ children, className, density = "pointer", id, role, tone = "help" }: FieldHintProps) => (
  <Text
    // `role` is outside TextProps on purpose (a readout is not a landmark); a hint is the exception.
    {...({ role } as object)}
    as={isWeb ? "p" : undefined}
    className={className}
    density={density}
    id={id}
    textRole="hint"
    tone={tone === "error" ? "danger" : "secondary"}
  >
    {children}
  </Text>
);

// The amber focus ring around a checkbox, switch or radio box, outside a gap so it never touches
// the box's own border.
const boxFocusRing = (focused: boolean, density: Density) =>
  focused
    ? ({
        outlineColor: "actionFocus",
        outlineOffset: 2,
        outlineStyle: "solid",
        outlineWidth: density === "tv" ? 3 : 2,
      } as const)
    : {};

type ToggleProps = {
  checked: boolean;
  density?: Density;
  description?: string;
  disabled?: boolean;
  kind?: "checkbox" | "switch";
  label: string;
  /**
   * Announce the label without printing it, for a control beside content that already shows the
   * same words (a row's select box next to the row's own title). The accessible name stays `label`.
   */
  labelHidden?: boolean;
  onCheckedChange: (checked: boolean) => void;
};

const Toggle = ({
  checked,
  density = "pointer",
  description,
  disabled = false,
  kind = "checkbox",
  label,
  labelHidden = false,
  onCheckedChange,
}: ToggleProps) => {
  const switchWidth = density === "tv" ? 72 : density === "touch" ? 56 : 48;
  const controlSize = density === "tv" ? 36 : density === "touch" ? 28 : 24;
  const [hasFocus, setFocused] = useState(false);
  const behindModal = useBehindModal();
  const focused = hasFocus && !behindModal;
  return (
    <Pressable
      accessibilityLabel={label}
      accessibilityRole={kind}
      accessibilityState={{ checked, disabled }}
      aria-checked={checked}
      aria-disabled={disabled || undefined}
      disabled={disabled}
      onBlur={() => setFocused(false)}
      onFocus={() => setFocused(true)}
      onPress={() => onCheckedChange(!checked)}
      style={{
        alignItems: "center",
        flexDirection: "row",
        gap: semanticSpace.control,
        minHeight: semanticTargets[density],
        opacity: disabled ? 0.55 : 1,
      }}
    >
      {/* Checked is a selection, so it is neutral-pressed (#1909): elevated fill with a foreground
          border and mark. Amber is the focus ring around the box, outside a gap. */}
      <Surface
        alignItems={kind === "switch" ? (checked ? "flex-end" : "flex-start") : "center"}
        backgroundColor={checked ? "surfaceElevated" : "surfaceCanvas"}
        borderColor={checked ? "contentPrimary" : "borderControl"}
        borderRadius={kind === "switch" ? "round" : "control"}
        height={controlSize}
        justifyContent="center"
        padding={kind === "switch" ? 3 : 0}
        width={kind === "switch" ? switchWidth : controlSize}
        {...boxFocusRing(focused, density)}
      >
        {kind === "switch" ? (
          <View
            backgroundColor={checked ? "contentPrimary" : "contentSecondary"}
            borderRadius="round"
            height={controlSize - 8}
            width={controlSize - 8}
          />
        ) : checked ? (
          <Icon
            decorative
            glyph={icons.success}
            size={density === "tv" ? "control" : "compact"}
            tone="content"
          />
        ) : null}
      </Surface>
      {labelHidden ? null : (
        <View flex={1} gap={2}>
          <Text density={density} textRole="label">
            {label}
          </Text>
          {description ? (
            <Text density={density} textRole="hint">
              {description}
            </Text>
          ) : null}
        </View>
      )}
    </Pressable>
  );
};

type ChoiceOption<Value extends string> = {
  description?: string;
  disabled?: boolean;
  label: string;
  value: Value;
};

type ChoiceGroupProps<Value extends string> = {
  density?: Density;
  label: string;
  onValueChange: (value: Value) => void;
  options: readonly ChoiceOption<Value>[];
  value: Value;
};

const ChoiceRow = ({
  checked,
  density,
  onPress,
  option,
}: {
  checked: boolean;
  density: Density;
  onPress: () => void;
  option: ChoiceOption<string>;
}) => {
  const [hasFocus, setFocused] = useState(false);
  const behindModal = useBehindModal();
  return (
    <Pressable
      accessibilityLabel={option.label}
      accessibilityRole="radio"
      accessibilityState={{ checked, disabled: option.disabled }}
      aria-checked={checked}
      aria-disabled={option.disabled || undefined}
      disabled={option.disabled}
      onBlur={() => setFocused(false)}
      onFocus={() => setFocused(true)}
      onPress={onPress}
      style={{
        alignItems: "center",
        flexDirection: "row",
        gap: semanticSpace.control,
        minHeight: semanticTargets[density],
        opacity: option.disabled ? 0.55 : 1,
      }}
    >
      <Surface
        alignItems="center"
        // A chosen option is neutral-pressed (#1909), like a checked box; amber is the focus ring.
        backgroundColor={checked ? "surfaceElevated" : "surfaceCanvas"}
        borderColor={checked ? "contentPrimary" : "borderControl"}
        borderRadius="round"
        height={density === "tv" ? 36 : 24}
        justifyContent="center"
        width={density === "tv" ? 36 : 24}
        {...boxFocusRing(hasFocus && !behindModal, density)}
      >
        {checked ? (
          <View backgroundColor="contentPrimary" borderRadius="round" height="45%" width="45%" />
        ) : null}
      </Surface>
      <View flex={1} gap={2}>
        <Text density={density} textRole="label">
          {option.label}
        </Text>
        {option.description ? (
          <Text density={density} textRole="hint">
            {option.description}
          </Text>
        ) : null}
      </View>
    </Pressable>
  );
};

const ChoiceGroup = <Value extends string>({
  density = "pointer",
  label,
  onValueChange,
  options,
  value,
}: ChoiceGroupProps<Value>) => (
  <View aria-label={label} role="radiogroup" gap="inline">
    <Text density={density} textRole="label">
      {label}
    </Text>
    {options.map((option) => (
      <ChoiceRow
        checked={option.value === value}
        density={density}
        key={option.value}
        onPress={() => onValueChange(option.value)}
        option={option}
      />
    ))}
  </View>
);

export type {
  ActionProps,
  ActionVariant,
  ChoiceGroupProps,
  ChoiceOption,
  FieldHintProps,
  FieldProps,
  ToggleProps,
};
export { Action, ChoiceGroup, Field, FieldHint, Toggle };
