import { View } from "@tamagui/core";
import { type ReactNode, useCallback, useEffect, useRef, useState } from "react";
import { Pressable, ScrollView, type ScrollViewInstance } from "react-native";

import { useFocusLook } from "../focus-look";
import { Icon } from "../icon";
import { icons } from "../icons";
import { Action, ChoiceGroup } from "../interaction";
import { boxFocusRing } from "../interaction/box-focus-ring";
import { Surface } from "../primitives";
import { Text } from "../text";
import { type Density, iconography, semanticRadius, semanticSpace, semanticTargets } from "../tokens";

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
    | { appearance: "underline"; options: readonly UnderlineTabOption<Value>[] }
  );

// One `underline` tab: the label and its count on a 2px rule that is the foreground colour when
// selected and clear otherwise. The rule overlaps the list's own 1px rule, so the two read as one.
// A hover lifts an unselected label to the foreground, as the dialog's close does.
const UnderlineTab = <Value extends string>({
  density,
  onLayout,
  onPress,
  option,
  selected,
}: {
  density: Density;
  /** Where the tab sits in the list, so the list can scroll it into view. */
  onLayout: (layout: { width: number; x: number }) => void;
  onPress: () => void;
  option: UnderlineTabOption<Value>;
  selected: boolean;
}) => {
  const { focused, ...focusHandlers } = useFocusLook();
  const [hovered, setHovered] = useState(false);
  const disabled = option.disabled ?? false;
  const lit = selected || (hovered && !disabled);
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
        borderBottomColor={selected ? "contentPrimary" : "transparent"}
        borderBottomWidth={2}
        justifyContent="flex-end"
        minHeight={semanticTargets[density]}
        paddingBottom={semanticSpace.inline}
        {...boxFocusRing(focused, density)}
      >
        {/* The count is nested in the label's line, so the two share a baseline and one name. */}
        <Text density={density} numberOfLines={1} strong textRole="body" tone={lit ? "primary" : "secondary"}>
          {option.label}
          {option.count ? (
            <Text density={density} textRole="caption" tone="secondary">
              {` ${option.count}`}
            </Text>
          ) : null}
        </Text>
      </View>
    </Pressable>
  );
};

// A pointer's way to the tabs past an edge, and the sign that there are more: a chevron on the
// canvas at that edge, above the rule. A press scrolls most of a view's width. Keyboard and screen
// reader users move between tabs with the arrow keys, so it is hidden from them.
// It reaches over the scroller's ring gutter (`inset`), so no clipped text shows beside it.
const EdgeChevron = ({
  density,
  inset,
  onPress,
  side,
}: {
  density: Density;
  inset: number;
  onPress: () => void;
  side: "end" | "start";
}) => (
  <Pressable
    aria-hidden
    focusable={false}
    onPress={onPress}
    style={{ bottom: 1, position: "absolute", top: 0, [side === "end" ? "right" : "left"]: -inset }}
    tabIndex={-1}
  >
    <View
      alignItems="center"
      backgroundColor="surfaceCanvas"
      height="100%"
      justifyContent="center"
      paddingHorizontal={2}
    >
      <Icon
        decorative
        glyph={side === "end" ? icons.later : icons.earlier}
        size={density === "tv" ? "control" : "compact"}
        tone="muted"
      />
    </View>
  </Pressable>
);

// The underline list never wraps: when its tabs do not fit (large type, a TV, a phone) it scrolls
// sideways. The selected tab is scrolled into view when it is set, a tab that takes focus scrolls
// into view, and a chevron at an edge says more tabs lie past it. The shared rule runs the list's
// full width. The scroller reaches the focus ring's width past the list on every side and pads it
// back in, so it never clips a tab's ring and the list's place and height do not change.
const UnderlineTabs = <Value extends string>({
  density = "pointer",
  label,
  onValueChange,
  options,
  value,
}: TabsBaseProps<Value> & { options: readonly UnderlineTabOption<Value>[] }) => {
  const ring = (density === "tv" ? 3 : 2) + 2;
  // An edge chevron's width: its glyph and its side padding.
  const chevron = iconography.size[density === "tv" ? "control" : "compact"] + 4;
  const scroller = useRef<ScrollViewInstance>(null);
  const tabs = useRef(new Map<Value, { width: number; x: number }>());
  const [view, setView] = useState({ content: 0, viewport: 0, x: 0 });

  const reveal = useCallback(() => {
    const tab = tabs.current.get(value);
    if (!tab || view.viewport === 0) return;
    // Room for the tab, its ring and a chevron beside it at either edge.
    const start = tab.x - chevron;
    const end = tab.x + tab.width + 2 * ring + chevron;
    if (start >= view.x && end <= view.x + view.viewport) return;
    scroller.current?.scrollTo({ animated: false, x: Math.max(0, end - view.viewport) });
  }, [chevron, ring, value, view.viewport, view.x]);
  // Only a change of selection or of the viewport scrolls; the reader's own scrolling does not.
  // biome-ignore lint/correctness/useExhaustiveDependencies: reveal reads the scroll offset but must not run on it.
  useEffect(reveal, [value, view.viewport]);

  const page = (direction: 1 | -1) =>
    scroller.current?.scrollTo({ animated: true, x: Math.max(0, view.x + direction * view.viewport * 0.8) });
  const overflows = view.content > view.viewport + 1;

  return (
    <View position="relative">
      <ScrollView
        contentContainerStyle={{ flexGrow: 1, padding: ring }}
        horizontal
        onContentSizeChange={(content) => setView((now) => ({ ...now, content }))}
        onLayout={(event) => {
          const viewport = event.nativeEvent.layout.width;
          setView((now) => ({ ...now, viewport }));
        }}
        onScroll={(event) => {
          const x = event.nativeEvent.contentOffset.x;
          setView((now) => ({ ...now, x }));
        }}
        ref={scroller}
        scrollEventThrottle={16}
        showsHorizontalScrollIndicator={false}
        style={{ flexGrow: 0, margin: -ring }}
      >
        <View
          aria-label={label}
          borderBottomColor="borderDecorative"
          borderBottomWidth={1}
          flexDirection="row"
          flexGrow={1}
          gap={semanticSpace.section}
          role="tablist"
        >
          {options.map((option) => (
            <UnderlineTab
              density={density}
              key={option.value}
              onLayout={(layout) => {
                tabs.current.set(option.value, layout);
                if (option.value === value) reveal();
              }}
              onPress={() => onValueChange(option.value)}
              option={option}
              selected={option.value === value}
            />
          ))}
        </View>
      </ScrollView>
      {overflows && view.x > 1 ? (
        <EdgeChevron density={density} inset={ring} onPress={() => page(-1)} side="start" />
      ) : null}
      {overflows && view.x + view.viewport < view.content - 1 ? (
        <EdgeChevron density={density} inset={ring} onPress={() => page(1)} side="end" />
      ) : null}
    </View>
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
