import { Text as TamaguiText, View } from "@tamagui/core";
import { useState } from "react";
import { Pressable } from "react-native";

import { Icon } from "../icon";
import { type IconName, icons } from "../icons";
import { useViewportInsets } from "../viewport";

type TabBarIdiom = "ios" | "material";

type TabBarItem<Value extends string> = {
  /** Only read when `kind` is "disclosure": whether the thing it opens is currently open. */
  ariaExpanded?: boolean;
  /** A plain dot (no numeral): web's Requests tab (#1785), matching today's icon-rail dot. */
  badge?: boolean;
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
  ios: { background: "$surfaceCanvas", glyph: "control", height: 49, hairline: 1, label: 10, top: 8 },
  material: {
    background: "$surfaceRaised",
    glyph: "navigation",
    height: 80,
    hairline: 0,
    label: 12,
    top: 12,
  },
} as const;

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
  return (
    <View
      aria-label={accessibilityLabel}
      backgroundColor={style.background}
      borderTopColor="$borderDecorative"
      borderTopWidth={style.hairline}
      flexDirection="row"
      paddingBottom={insets.bottom}
      paddingLeft={insets.left}
      paddingRight={insets.right}
      role="tablist"
    >
      {items.map((item) => {
        const disclosure = item.kind === "disclosure";
        const current = !disclosure && item.value === selected;
        return (
          <Pressable
            accessibilityLabel={item.label}
            accessibilityRole={disclosure ? "button" : "tab"}
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
              backgroundColor={idiom === "material" && current ? "$borderDecorative" : "$transparent"}
              borderColor={focused === item.value ? "$actionFocus" : "$transparent"}
              borderRadius={idiom === "material" ? "$round" : 8}
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
              {item.badge ? (
                <View
                  aria-hidden
                  // `--color-suggest` (@loomarr/tokens): the same magenta the web rail's
                  // numeral badge already uses for "needs you" counts.
                  backgroundColor="#D6409F"
                  borderRadius="$round"
                  height={7}
                  position="absolute"
                  right={-2}
                  top={-2}
                  width={7}
                />
              ) : null}
            </View>
            <TamaguiText
              color={current ? "$contentPrimary" : "$contentSecondary"}
              fontFamily="$body"
              fontSize={style.label}
              fontWeight={current ? "600" : "500"}
              lineHeight={Math.round(style.label * 1.3)}
            >
              {item.label}
            </TamaguiText>
          </Pressable>
        );
      })}
    </View>
  );
};

export type { TabBarIdiom, TabBarItem, TabBarProps };
export { TabBar };
