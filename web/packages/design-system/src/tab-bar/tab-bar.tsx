import { View } from "@tamagui/core";
import { useState } from "react";
import { Platform, Pressable } from "react-native";

import { Icon } from "../icon";
import { type IconName, icons } from "../icons";
import { lineHeightPx } from "../text";
import { TamaguiText } from "../text-base";
import { brandChroma } from "../tokens";
import { useViewportInsets } from "../viewport";

type TabBarIdiom = "ios" | "material";

type TabBarItem<Value extends string> = {
  /** Only read when `kind` is "disclosure": whether the thing it opens is currently open. */
  ariaExpanded?: boolean;
  /**
   * A count over the glyph (the phone's Requests: what needs the person), or `true` for a plain
   * dot with no numeral (web's bar, #1785, matching its icon-rail dot). Omitted, 0 or `false`
   * draws nothing.
   */
  badge?: boolean | number;
  /**
   * What a numbered badge counts, said after the number in the tab's name ("Library, 4 need you").
   * `one` is the words for a count of exactly 1. Defaults to "needs attention" / "need attention".
   */
  badgeWords?: { one: string; other: string };
  icon: IconName;
  /**
   * "tab" (default): a `role="tab"` destination, lit when `selected` matches it. "disclosure":
   * web's trailing **More** button (#1785) for destinations that don't fit — it opens a sheet
   * rather than switching the selected value, so it carries `aria-haspopup`/`aria-expanded`
   * instead of `role="tab"`/`aria-selected`, and is never lit.
   */
  kind?: "disclosure" | "tab";
  label: string;
  value: Value;
};

type TabBarProps<Value extends string> = {
  accessibilityLabel: string;
  /** iPhone's bar (#1659 native mock 5d, 5e) or Android's Material bar with its pill (5f, 5g). */
  idiom: TabBarIdiom;
  items: readonly TabBarItem<Value>[];
  onSelect: (value: Value) => void;
  selected: Value;
};

// Measured from the native mock. iPhone: the canvas under a hairline, 8 above a 24 glyph, a 10 pt
// label, 49 pt of bar over the home indicator. Android: raised, 12 above a 64×32 pill holding a 22
// glyph, a 12 pt label, 80 dp of bar over the gesture strip.
const idioms = {
  ios: { background: "surfaceCanvas", glyph: "control", height: 49, hairline: 1, label: 10, top: 8 },
  material: {
    background: "surfaceRaised",
    glyph: "navigation",
    height: 80,
    hairline: 0,
    label: 12,
    top: 12,
  },
} as const;

const attentionWords = { one: "needs attention", other: "need attention" };
// A constant rather than JSX text, which a formatter reflow can drop.
const wordSpace = " ";

/**
 * A phone's bottom navigation, edge to edge: pass it as a Screen's `footer`. It carries the bottom
 * safe area itself. Each item is a whole-column target at least 49 pt tall.
 */
const TabBar = <Value extends string>({
  accessibilityLabel,
  idiom,
  items,
  onSelect,
  selected,
}: TabBarProps<Value>) => {
  const insets = useViewportInsets();
  const [focused, setFocused] = useState<Value | null>(null);
  const style = idioms[idiom];
  // `role="tablist"` requires its direct children to be `role="tab"` — ARIA's
  // aria-required-children. The "more" disclosure button is a `role="button"`, so it sits
  // OUTSIDE the tablist as a flex sibling, not as one more item inside it.
  const tabItems = items.filter((item) => item.kind !== "disclosure");
  const disclosureItems = items.filter((item) => item.kind === "disclosure");
  const renderItem = (item: TabBarItem<Value>) => {
    const disclosure = item.kind === "disclosure";
    const current = !disclosure && item.value === selected;
    const count = typeof item.badge === "number" && item.badge > 0 ? item.badge : 0;
    // The name repeats the number as drawn, so what a sighted person reads is in what is spoken.
    const shown = count > 99 ? "99+" : String(count);
    const words = item.badgeWords ?? attentionWords;
    return (
      <Pressable
        accessibilityLabel={
          count ? `${item.label}, ${shown} ${count === 1 ? words.one : words.other}` : item.label
        }
        // iOS has no tab trait: a UITabBar item is a button, which VoiceOver calls a tab inside the
        // bar's tab-bar trait. Android and the web have a real tab role.
        accessibilityRole={disclosure || Platform.OS === "ios" ? "button" : "tab"}
        accessibilityState={disclosure ? undefined : { selected: current }}
        aria-expanded={disclosure ? item.ariaExpanded : undefined}
        aria-haspopup={disclosure ? "dialog" : undefined}
        aria-selected={disclosure ? undefined : current}
        key={item.value}
        onBlur={() => setFocused(null)}
        onFocus={() => setFocused(item.value)}
        onPress={() => onSelect(item.value)}
        style={{ alignItems: "center", flex: 1, gap: 4, minHeight: style.height, paddingTop: style.top }}
      >
        {/* The pill is Material's selection mark; on iPhone the same box only draws the focus ring. */}
        <View
          alignItems="center"
          backgroundColor={idiom === "material" && current ? "borderDecorative" : "transparent"}
          borderColor={focused === item.value ? "actionFocus" : "transparent"}
          borderRadius={idiom === "material" ? "round" : 8}
          borderWidth={2}
          height={idiom === "material" ? 32 : 28}
          justifyContent="center"
          width={64}
        >
          <Icon
            decorative
            glyph={icons[item.icon]}
            size={style.glyph}
            tone={current ? "content" : "secondary"}
          />
          {item.badge === true ? (
            <View
              aria-hidden
              // The brand's suggest magenta (`--color-suggest`): the same one the web rail's
              // numeral badge already uses for "needs you" counts.
              backgroundColor={brandChroma[4]}
              borderRadius="round"
              height={7}
              position="absolute"
              right={-2}
              top={-2}
              width={7}
            />
          ) : null}
        </View>
        <TamaguiText
          color={current ? "contentPrimary" : "contentSecondary"}
          fontFamily="body"
          fontSize={style.label}
          fontWeight={current ? "600" : "500"}
          lineHeight={lineHeightPx(Math.round(style.label * 1.3))}
        >
          {item.label}
        </TamaguiText>
        {/*
          The number follows the label in the tree, with a space between, so the tab's text reads
          "Library 4" and its name can contain it. Without the space it reads "4Library", one word
          that no name starting with the label contains. Both are out of flow: the space draws
          nothing, and the number sits over the glyph's pill, 8 right of the column's centre.
        */}
        {count ? (
          <>
            <TamaguiText position="absolute">{wordSpace}</TamaguiText>
            <View
              alignItems="center"
              backgroundColor="stateLive"
              borderRadius="round"
              height={16}
              justifyContent="center"
              left="50%"
              marginLeft={8}
              minWidth={16}
              paddingHorizontal={4}
              position="absolute"
              top={style.top}
            >
              <TamaguiText color="contentInverse" fontFamily="body" fontSize={10} fontWeight="700">
                {shown}
              </TamaguiText>
            </View>
          </>
        ) : null}
      </Pressable>
    );
  };
  return (
    <View
      backgroundColor={style.background}
      borderTopColor="borderDecorative"
      borderTopWidth={style.hairline}
      flexDirection="row"
      paddingBottom={insets.bottom}
      paddingLeft={insets.left}
      paddingRight={insets.right}
    >
      <View
        aria-label={accessibilityLabel}
        flex={tabItems.length}
        flexDirection="row"
        // iOS has no tab-list trait: VoiceOver announces "Tab, 2 of 3" only for the items of a
        // container carrying the tab-bar trait, which React Native maps from "tabbar". Android
        // throws on "tabbar" and reads "tablist" as its TabWidget, as the web reads ARIA's.
        {...(Platform.OS === "ios" ? { accessibilityRole: "tabbar" as const } : { role: "tablist" as const })}
      >
        {tabItems.map(renderItem)}
      </View>
      {disclosureItems.map(renderItem)}
    </View>
  );
};

export type { TabBarIdiom, TabBarItem, TabBarProps };
export { TabBar };
