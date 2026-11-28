import {
  Action,
  Icon,
  icons,
  Menu,
  MenuAction,
  MenuContent,
  MenuTrigger,
  Text,
} from "@loomarr/design-system";
import { Children, Fragment, type ReactNode } from "react";
import { Platform } from "react-native";

import { Line } from "../library-poster";

// A native screen this wide is tapped; the web at this width is driven by a pointer.
const PHRASE_DENSITY = Platform.OS === "web" ? "pointer" : "touch";

// A phrase that opens a menu of choices. Its name is its words, then what it picks.
const PhraseMenu = <Value extends string>({
  label,
  options,
  value,
  words,
  onValueChange,
}: {
  label: string;
  options: readonly { label: string; value: Value }[];
  value: Value;
  /** What the phrase reads as in the sentence. */
  words: string;
  onValueChange: (value: Value) => void;
}) => (
  <Menu>
    <MenuTrigger>
      <Action
        accessibilityLabel={`${words}, ${label}`}
        density={PHRASE_DENSITY}
        icon="channelDown"
        iconPlacement="end"
        variant="ghost"
      >
        {words}
      </Action>
    </MenuTrigger>
    <MenuContent align="start">
      {options.map((option) => (
        <MenuAction
          density={PHRASE_DENSITY}
          icon={
            option.value === value ? <Icon decorative glyph={icons.selected} size="compact" /> : undefined
          }
          key={option.value}
          onSelect={() => onValueChange(option.value)}
        >
          {option.label}
        </MenuAction>
      ))}
    </MenuContent>
  </Menu>
);

// The parts of one quiet line, a dot between each. A part that is absent takes its dot with it.
const Sentence = ({ children }: { children: ReactNode }) => (
  <Line>
    {Children.toArray(children).map((part, index) => (
      // biome-ignore lint/suspicious/noArrayIndexKey: toArray keys the parts; the dot follows its part.
      <Fragment key={index}>
        {index > 0 ? (
          <Text aria-hidden textRole="compact" tone="secondary">
            ·
          </Text>
        ) : null}
        {part}
      </Fragment>
    ))}
  </Line>
);

export { PHRASE_DENSITY, PhraseMenu, Sentence };
