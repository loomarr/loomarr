import { useTheme, View } from "@tamagui/core";
import { type ReactNode, useCallback, useEffect, useRef, useState } from "react";
import { Platform, Pressable, ScrollView, type ScrollViewInstance, type ViewStyle } from "react-native";

import { useFocusLook } from "../focus-look";
import { Icon } from "../icon";
import { icons } from "../icons";
import { Action, ChoiceGroup } from "../interaction";
import { boxFocusRing } from "../interaction/box-focus-ring";
import { useReducedMotionPreference } from "../motion/use-reduced-motion";
import { Surface } from "../primitives";
import { Text } from "../text";
import { type Density, semanticRadius, semanticSpace, semanticTargets } from "../tokens";

type TabOption<Value extends string> = {
  disabled?: boolean;
  label: string;
  value: Value;
};

/** An `underline` tab, which may carry a count beside its label. */
type UnderlineTabOption<Value extends string> = TabOption<Value> & {
  /** The count as drawn beside the label ("3,412"). */
  count?: string;
  /**
   * The count as spoken, in place of the bare number ("3,412 titles"): the tab is then named
   * "All, 3,412 titles". Without it the tab's name is its label and count as drawn.
   */
  countLabel?: string;
};

interface TabsBaseProps<Value extends string> {
  density?: Density;
  label: string;
  onValueChange: (value: Value) => void;
  value: Value;
}

/**
 * `action`, the default, draws each tab as an outlined Action. `underline` sets the words on a
 * shared rule with the selected one underlined in the foreground colour, for a page's sections (the
 * Library's All, Movies, Shows), and only it draws a count, so only it accepts one. Amber stays the
 * focus ring only.
 */
type TabsProps<Value extends string> = TabsBaseProps<Value> &
  (
    | {
        appearance?: "action";
        options: readonly (TabOption<Value> & { count?: never; countLabel?: never })[];
      }
    | {
        appearance: "underline";
        options: readonly UnderlineTabOption<Value>[];
        /** `section` draws the words at section size (20/600), as a page's top shelf. */
        size?: "section";
      }
  );

// Where a tab sits along its list, from the list's start.
type TabPlace = { width: number; x: number };

// One `underline` tab: the label and its count on a 2px rule that is the foreground colour when
// selected and clear otherwise. The rule overlaps the list's own 1px rule, so the two read as one.
// A hover lifts an unselected label to the foreground, as the dialog's close does. The focus ring
// is drawn inside the tab's own box, so the scroller never clips it, with air at each side
// (`tabAir`) between the ring and the label; the list's gap gives that air back, so labels keep
// their spacing.
const tabAir = (density: Density) => (density === "tv" ? 3 : 2) + 2;

const UnderlineTab = <Value extends string>({
  density,
  onLayout,
  onPress,
  option,
  selected,
  size,
}: {
  density: Density;
  size?: "section";
  /** Where the tab sits in the list, so the list can bring it into view. */
  onLayout: (layout: TabPlace) => void;
  onPress: () => void;
  option: UnderlineTabOption<Value>;
  selected: boolean;
}) => {
  const { focused, ...focusHandlers } = useFocusLook();
  const [hovered, setHovered] = useState(false);
  const disabled = option.disabled ?? false;
  const lit = selected || (hovered && !disabled);
  const ring = boxFocusRing(focused, density);
  return (
    <Pressable
      accessibilityLabel={
        option.count && option.countLabel ? `${option.label}, ${option.countLabel}` : undefined
      }
      onHoverIn={() => setHovered(true)}
      onHoverOut={() => setHovered(false)}
      accessibilityRole="tab"
      accessibilityState={{ disabled, selected }}
      aria-disabled={disabled || undefined}
      aria-selected={selected}
      disabled={disabled}
      {...focusHandlers}
      onLayout={(event) => onLayout({ width: event.nativeEvent.layout.width, x: event.nativeEvent.layout.x })}
      onPress={onPress}
      style={{ marginBottom: -1, opacity: disabled ? 0.55 : 1, outlineStyle: "solid", outlineWidth: 0 }}
      tabIndex={disabled ? -1 : selected ? 0 : -1}
    >
      <View
        paddingHorizontal={tabAir(density)}
        {...ring}
        {...("outlineWidth" in ring ? { outlineOffset: -(ring.outlineWidth ?? 0) } : null)}
      >
        <View
          borderBottomColor={selected ? "contentPrimary" : "transparent"}
          borderBottomWidth={2}
          justifyContent="flex-end"
          minHeight={semanticTargets[density]}
          paddingBottom={semanticSpace.inline}
          // A finger needs the target as wide as it is tall, and a short word such as "All" is not.
          {...(density === "touch" ? { alignItems: "center", minWidth: semanticTargets.touch } : null)}
        >
          {/* The count is nested in the label's line, so the two share a baseline and one name. */}
          <Text
            density={density}
            numberOfLines={1}
            strong
            textRole={size === "section" ? "headline" : "body"}
            tone={lit ? "primary" : "secondary"}
          >
            {option.label}
            {option.count ? (
              <Text density={density} textRole="caption" tone="secondary">
                {` ${option.count}`}
              </Text>
            ) : null}
          </Text>
        </View>
      </View>
    </Pressable>
  );
};

// The underline list never wraps: when its tabs do not fit (large type, a TV, a phone) it scrolls
// sideways and shows its scroll bar. The shared rule runs the list's full width. The selected tab
// is brought into view once when the list mounts and once each time the selection changes, and
// never otherwise: a count, a font or the window changing size leaves the reader's scroll alone.
const UnderlineTabs = <Value extends string>({
  density = "pointer",
  label,
  onValueChange,
  options,
  size,
  value,
}: TabsBaseProps<Value> & { options: readonly UnderlineTabOption<Value>[]; size?: "section" }) => {
  const reducedMotion = useReducedMotionPreference();
  const animated = useRef(false);
  animated.current = reducedMotion === false;
  const scroller = useRef<ScrollViewInstance>(null);
  const tabs = useRef(new Map<Value, TabPlace>());
  const viewport = useRef(0);
  const offset = useRef(0);
  // The selection still to bring into view: it waits until both it and the list have been measured.
  const pending = useRef<Value | null>(value);
  // A browser draws a thin bar in the theme's muted colour on a clear track, only while the list
  // overflows; React Native's own indicator needs no styling.
  const theme = useTheme();
  const scrollBar = (
    Platform.OS === "web"
      ? { scrollbarColor: `${theme.contentMuted.val} transparent`, scrollbarWidth: "thin" }
      : {}
  ) as ViewStyle;

  const reveal = useCallback(() => {
    const tab = pending.current === null ? undefined : tabs.current.get(pending.current);
    if (!tab || viewport.current === 0) return;
    pending.current = null;
    const end = tab.x + tab.width;
    const x =
      tab.x < offset.current
        ? tab.x
        : end > offset.current + viewport.current
          ? end - viewport.current
          : null;
    if (x !== null) scroller.current?.scrollTo({ animated: animated.current, x });
  }, []);
  useEffect(() => {
    pending.current = value;
    reveal();
  }, [reveal, value]);

  return (
    <ScrollView
      contentContainerStyle={{ flexGrow: 1 }}
      horizontal
      onLayout={(event) => {
        viewport.current = event.nativeEvent.layout.width;
        reveal();
      }}
      onScroll={(event) => {
        offset.current = event.nativeEvent.contentOffset.x;
      }}
      ref={scroller}
      scrollEventThrottle={16}
      style={{ flexGrow: 0, ...scrollBar }}
    >
      <View
        aria-label={label}
        borderBottomColor="borderDecorative"
        borderBottomWidth={1}
        flexDirection="row"
        flexGrow={1}
        // Touch type is larger on the narrowest screens, so its labels sit closer: a phone then
        // shows a page's four sections without scrolling.
        gap={(density === "touch" ? semanticSpace.control : semanticSpace.section) - 2 * tabAir(density)}
        role="tablist"
      >
        {options.map((option) => (
          <UnderlineTab
            density={density}
            key={option.value}
            onLayout={(place) => {
              tabs.current.set(option.value, place);
              reveal();
            }}
            onPress={() => onValueChange(option.value)}
            option={option}
            selected={option.value === value}
            size={size}
          />
        ))}
      </View>
    </ScrollView>
  );
};

const Tabs = <Value extends string>(props: TabsProps<Value>) => {
  if (props.appearance === "underline") return <UnderlineTabs {...props} />;
  const { density = "pointer", label, onValueChange, options, value } = props;
  return (
    <View aria-label={label} flexDirection="row" gap="inline" role="tablist">
      {options.map((option) => {
        const selected = option.value === value;
        return (
          <Action
            accessibilityRole="tab"
            density={density}
            disabled={option.disabled}
            key={option.value}
            onPress={() => onValueChange(option.value)}
            selected={selected}
            tabIndex={selected ? 0 : -1}
            tone="secondary"
          >
            {option.label}
          </Action>
        );
      })}
    </View>
  );
};

type MenuItem<Value extends string> = {
  disabled?: boolean;
  label: string;
  tone?: "danger" | "primary" | "secondary";
  value: Value;
};

interface MenuListProps<Value extends string> {
  density?: Density;
  items: readonly MenuItem<Value>[];
  label: string;
  onSelect: (value: Value) => void;
}

const MenuList = <Value extends string>({
  density = "pointer",
  items,
  label,
  onSelect,
}: MenuListProps<Value>) => (
  <Surface aria-label={label} gap="inline" padding="inline" role="menu">
    {items.map((item) => (
      <Action
        accessibilityRole="menuitem"
        density={density}
        disabled={item.disabled}
        key={item.value}
        onPress={() => onSelect(item.value)}
        tone={item.tone ?? "secondary"}
      >
        {item.label}
      </Action>
    ))}
  </Surface>
);

type SelectOption<Value extends string> = {
  description?: string;
  disabled?: boolean;
  label: string;
  value: Value;
};

interface SelectControlProps<Value extends string> {
  density?: Density;
  disabled?: boolean;
  /**
   * `field` draws the closed control as a full-width form box with a chevron (the web forms'
   * select), for a row that already prints its own label: the label is then announced, not shown.
   */
  appearance?: "button" | "field";
  label: string;
  onOpenChange: (open: boolean) => void;
  onValueChange: (value: Value) => void;
  open: boolean;
  options: readonly SelectOption<Value>[];
  // What the closed control says before anything is chosen ("Choose one account").
  placeholder?: string;
  value: Value;
}

const SelectControl = <Value extends string>({
  appearance = "button",
  density = "pointer",
  disabled = false,
  label,
  onOpenChange,
  onValueChange,
  open,
  options,
  placeholder = "Choose an option",
  value,
}: SelectControlProps<Value>) => {
  const selected = options.find((option) => option.value === value);
  return (
    <View gap="inline" width="100%">
      {appearance === "field" ? (
        <Pressable
          accessibilityLabel={`${label}, ${selected?.label ?? "No selection"}`}
          accessibilityRole="button"
          aria-expanded={open}
          disabled={disabled}
          onPress={() => onOpenChange(!open)}
        >
          <Surface
            alignItems="center"
            backgroundColor="surfaceCanvas"
            borderColor="borderControl"
            borderRadius={semanticRadius.control}
            borderWidth={1}
            flexDirection="row"
            gap="inline"
            minHeight={semanticTargets[density]}
            opacity={disabled ? 0.62 : 1}
            paddingHorizontal={semanticSpace.control}
          >
            <View flex={1} minWidth={0}>
              {/* Left, like a form field: the web Pressable's button would otherwise centre it. */}
              <Text density={density} numberOfLines={1} textAlign="left" textRole="compact" tone="primary">
                {selected?.label ?? placeholder}
              </Text>
            </View>
            <Icon decorative glyph={icons.channelDown} size="compact" tone="secondary" />
          </Surface>
        </Pressable>
      ) : (
        <>
          <Text density={density} textRole="label">
            {label}
          </Text>
          <Action
            accessibilityLabel={`${label}, ${selected?.label ?? "No selection"}`}
            accessibilityRole="button"
            aria-expanded={open}
            density={density}
            disabled={disabled}
            onPress={() => onOpenChange(!open)}
            tone="secondary"
          >
            {selected?.label ?? placeholder}
          </Action>
        </>
      )}
      {open ? (
        <Surface padding="control">
          <ChoiceGroup
            density={density}
            label={`${label} options`}
            onValueChange={(next) => {
              onValueChange(next);
              onOpenChange(false);
            }}
            options={options}
            value={value}
          />
        </Surface>
      ) : null}
    </View>
  );
};

interface HintProps {
  children: ReactNode;
  content: string;
  density?: Density;
  visible: boolean;
}

const Hint = ({ children, content, density = "pointer", visible }: HintProps) => (
  <View alignItems="flex-start" gap="inline">
    {children}
    {visible ? (
      <Surface level="overlay" maxWidth={density === "tv" ? 520 : 320} padding="inline" role="tooltip">
        <Text density={density} textRole="metadata" tone="primary">
          {content}
        </Text>
      </Surface>
    ) : null}
  </View>
);

export type {
  HintProps,
  MenuItem,
  MenuListProps,
  SelectControlProps,
  SelectOption,
  TabOption,
  TabsProps,
  UnderlineTabOption,
};
export { Hint, MenuList, SelectControl, Tabs };
