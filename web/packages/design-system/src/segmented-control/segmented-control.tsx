import { Text as TamaguiText, View } from "@tamagui/core";
import { useState } from "react";
import { Pressable } from "react-native";

type SegmentOption<Value extends string> = {
  /** What the segment counts, shown beside its label and read with it; omitted draws and says nothing. */
  count?: number;
  disabled?: boolean;
  label: string;
  value: Value;
};

type SegmentedControlProps<Value extends string> = {
  /** Names the group to a screen reader, e.g. "Requests sections". */
  accessibilityLabel: string;
  onValueChange: (value: Value) => void;
  options: readonly SegmentOption<Value>[];
  value: Value;
};

// iPhone's segmented control, drawn from tokens: one rounded track holding equal-width segments, the
// selected one raised on a thumb. Native's is 32 pt; touch targets here are 44 pt, so the track is 48.
const SEGMENT_HEIGHT = 44;
const TRACK_PADDING = 2;

const segmentLabel = (option: SegmentOption<string>) =>
  option.count === undefined ? option.label : `${option.label}, ${option.count}`;

/**
 * A full-width, two-or-more segment choice for a phone screen: pass `options` in display order. Each
 * segment is a whole-width-share target at least 44 pt tall, announced as a tab with its selected state
 * and, when it has one, its count ("Needs you, 6").
 */
const SegmentedControl = <Value extends string>({
  accessibilityLabel,
  onValueChange,
  options,
  value,
}: SegmentedControlProps<Value>) => {
  const [focused, setFocused] = useState<Value | null>(null);
  return (
    <View
      aria-label={accessibilityLabel}
      backgroundColor="$surfaceRaised"
      borderColor="$borderDecorative"
      borderRadius="$control"
      borderWidth={1}
      flexDirection="row"
      gap={TRACK_PADDING}
      padding={TRACK_PADDING}
      role="tablist"
      width="100%"
    >
      {options.map((option) => {
        const selected = option.value === value;
        return (
          <Pressable
            accessibilityLabel={segmentLabel(option)}
            accessibilityRole="tab"
            accessibilityState={{ disabled: option.disabled ?? false, selected }}
            aria-selected={selected}
            disabled={option.disabled}
            key={option.value}
            onBlur={() => setFocused(null)}
            onFocus={() => setFocused(option.value)}
            onPress={() => onValueChange(option.value)}
            style={{ flex: 1, minHeight: SEGMENT_HEIGHT }}
          >
            <View
              alignItems="center"
              backgroundColor={selected ? "$borderDecorative" : "$transparent"}
              borderColor={focused === option.value ? "$actionFocus" : "$transparent"}
              borderRadius={8}
              borderWidth={2}
              flex={1}
              flexDirection="row"
              gap={4}
              justifyContent="center"
              minHeight={SEGMENT_HEIGHT}
              opacity={option.disabled ? 0.4 : 1}
              paddingHorizontal={4}
            >
              <TamaguiText
                color={selected ? "$contentPrimary" : "$contentSecondary"}
                fontFamily="$body"
                fontSize={13}
                fontWeight={selected ? "600" : "500"}
                numberOfLines={1}
              >
                {option.label}
              </TamaguiText>
              {option.count === undefined ? null : (
                <TamaguiText
                  color={selected ? "$contentPrimary" : "$contentSecondary"}
                  fontFamily="$body"
                  fontSize={13}
                  fontWeight="500"
                >
                  {option.count}
                </TamaguiText>
              )}
            </View>
          </Pressable>
        );
      })}
    </View>
  );
};

export type { SegmentedControlProps, SegmentOption };
export { SegmentedControl };
