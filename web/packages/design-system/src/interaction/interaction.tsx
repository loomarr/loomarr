import { isWeb, Text as TamaguiText, useTheme, View } from "@tamagui/core";
import type { ComponentProps, ComponentRef, ReactElement, ReactNode } from "react";
import { cloneElement, forwardRef, isValidElement, useState } from "react";
import { Pressable, TextInput } from "react-native";

import { Icon, type IconTone } from "../icon";
import { type IconName, icons } from "../icons";
import { Surface, Text } from "../primitives";
import {
  brandChroma,
  type Density,
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

const Action = forwardRef<ComponentRef<typeof Pressable>, ActionProps>(
  (
    {
      accessibilityState,
      children,
      density = "pointer",
      disabled = false,
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
    const [focused, setFocused] = useState(false);
    const isDisabled = disabled === true;
    const role = props.accessibilityRole ?? "button";
    const tv = density === "tv";
    const stacked = layout === "stacked" && icon !== undefined;
    const look = variant ? variantLook(theme)[variant] : undefined;
    const backgroundColor = look
      ? look.background
      : tone === "danger"
        ? theme.stateDanger.val
        : tone === "primary"
          ? theme.actionPrimary.val
          : selected
            ? theme.surfaceFocus.val
            : theme.surfaceElevated.val;
    const borderColor = look
      ? focused
        ? theme.actionFocus.val
        : look.border
      : focused || selected
        ? theme.actionFocus.val
        : tone === "danger"
          ? theme.stateDanger.val
          : tone === "primary"
            ? theme.actionPrimary.val
            : stacked
              ? backgroundColor
              : theme.borderControl.val;
    const textColor = variant ? variantTextColor(theme, variant) : undefined;
    const iconTone: IconTone = look
      ? look.iconTone
      : tone === "secondary" && !selected
        ? "secondary"
        : "inverse";

    const content =
      stacked && icon ? (
        <View alignItems="center" gap={3} paddingVertical={semanticSpace.inline}>
          <Icon
            decorative
            glyph={icons[icon]}
            size="default"
            tone={look ? look.iconTone : tone === "secondary" && !selected ? "content" : "inverse"}
          />
          <TamaguiText
            color={textColor ?? (tone === "secondary" ? "$contentSecondary" : "$contentInverse")}
            fontFamily="$body"
            fontSize={typography[density].cardMeta.size}
            lineHeight={typography[density].cardMeta.lineHeight}
            numberOfLines={1}
          >
            {children}
          </TamaguiText>
        </View>
      ) : icon ? (
        <View alignItems="center" flexDirection="row" gap="$inline">
          <Icon
            decorative
            glyph={icons[icon]}
            size={density === "tv" ? "touch" : "default"}
            tone={iconTone}
          />
          <TamaguiText
            color={textColor ?? (tone === "secondary" ? "$contentPrimary" : "$contentInverse")}
            fontFamily="$body"
            fontSize={tv ? typography.tv.data.size : typography[density].label.size}
            fontWeight={tv ? "400" : "700"}
            textDecorationLine={variant === "link" ? "underline" : undefined}
          >
            {children}
          </TamaguiText>
        </View>
      ) : (
        <TamaguiText
          color={textColor ?? (tone === "secondary" ? "$contentPrimary" : "$contentInverse")}
          fontFamily="$body"
          fontSize={tv ? typography.tv.data.size : typography[density].label.size}
          fontWeight={tv ? "400" : "700"}
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
        : focused
          ? tv
            ? 3
            : 4
          : tv
            ? 1
            : 2,
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
    <View gap="$inline" width="100%">
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
            fontFamily: typography.family.data.native,
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

type ToggleProps = {
  checked: boolean;
  density?: Density;
  description?: string;
  disabled?: boolean;
  kind?: "checkbox" | "switch";
  label: string;
  onCheckedChange: (checked: boolean) => void;
};

const Toggle = ({
  checked,
  density = "pointer",
  description,
  disabled = false,
  kind = "checkbox",
  label,
  onCheckedChange,
}: ToggleProps) => {
  const switchWidth = density === "tv" ? 72 : density === "touch" ? 56 : 48;
  const controlSize = density === "tv" ? 36 : density === "touch" ? 28 : 24;
  return (
    <Pressable
      accessibilityLabel={label}
      accessibilityRole={kind}
      accessibilityState={{ checked, disabled }}
      aria-checked={checked}
      aria-disabled={disabled || undefined}
      disabled={disabled}
      onPress={() => onCheckedChange(!checked)}
      style={{
        alignItems: "center",
        flexDirection: "row",
        gap: semanticSpace.control,
        minHeight: semanticTargets[density],
        opacity: disabled ? 0.55 : 1,
      }}
    >
      <Surface
        alignItems={kind === "switch" ? (checked ? "flex-end" : "flex-start") : "center"}
        backgroundColor={checked ? "$actionPrimary" : "$surfaceCanvas"}
        borderColor={checked ? "$actionPrimary" : "$borderControl"}
        borderRadius={kind === "switch" ? "$round" : "$control"}
        height={controlSize}
        justifyContent="center"
        padding={kind === "switch" ? 3 : 0}
        width={kind === "switch" ? switchWidth : controlSize}
      >
        {kind === "switch" ? (
          <View
            backgroundColor={checked ? "$contentInverse" : "$contentSecondary"}
            borderRadius="$round"
            height={controlSize - 8}
            width={controlSize - 8}
          />
        ) : checked ? (
          <Icon
            decorative
            glyph={icons.success}
            size={density === "tv" ? "control" : "compact"}
            tone="inverse"
          />
        ) : null}
      </Surface>
      <View flex={1} gap={2}>
        <Text density={density} textRole="label">
          {label}
        </Text>
        {description ? (
          <Text density={density} textRole="metadata">
            {description}
          </Text>
        ) : null}
      </View>
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

const ChoiceGroup = <Value extends string>({
  density = "pointer",
  label,
  onValueChange,
  options,
  value,
}: ChoiceGroupProps<Value>) => (
  <View aria-label={label} role="radiogroup" gap="$inline">
    <Text density={density} textRole="label">
      {label}
    </Text>
    {options.map((option) => {
      const checked = option.value === value;
      return (
        <Pressable
          accessibilityLabel={option.label}
          accessibilityRole="radio"
          accessibilityState={{ checked, disabled: option.disabled }}
          aria-checked={checked}
          aria-disabled={option.disabled || undefined}
          disabled={option.disabled}
          key={option.value}
          onPress={() => onValueChange(option.value)}
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
            backgroundColor={checked ? "$actionPrimary" : "$surfaceCanvas"}
            borderColor={checked ? "$actionPrimary" : "$borderControl"}
            borderRadius="$round"
            height={density === "tv" ? 36 : 24}
            justifyContent="center"
            width={density === "tv" ? 36 : 24}
          >
            {checked ? (
              <View backgroundColor="$contentInverse" borderRadius="$round" height="45%" width="45%" />
            ) : null}
          </Surface>
          <View flex={1} gap={2}>
            <Text density={density} textRole="label">
              {option.label}
            </Text>
            {option.description ? (
              <Text density={density} textRole="metadata">
                {option.description}
              </Text>
            ) : null}
          </View>
        </Pressable>
      );
    })}
  </View>
);

export type { ActionProps, ActionVariant, ChoiceGroupProps, ChoiceOption, FieldProps, ToggleProps };
export { Action, ChoiceGroup, Field, Toggle };
