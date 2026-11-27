import { View } from "@tamagui/core";
import { useState } from "react";
import { Pressable } from "react-native";

import { Text } from "../text";
import type { DetailsProps } from "./details.type";

// The native adapter: there is no `<details>` on a phone or a TV, so the summary is a button that
// shows or hides the content, announced as expanded or collapsed. It has no marker of its own.

const Details = ({
  children,
  defaultOpen = false,
  onToggle,
  open,
  summary,
  textRole = "compact",
  tone,
}: DetailsProps) => {
  const [state, setState] = useState(open ?? defaultOpen);
  const [given, setGiven] = useState(open);

  if (open !== given) {
    setGiven(open);
    if (open !== undefined) setState(open);
  }

  return (
    <View>
      <Pressable
        accessibilityRole="button"
        accessibilityState={{ expanded: state }}
        onPress={() => {
          setState(!state);
          onToggle?.(!state);
        }}
      >
        <Text textRole={textRole} tone={tone}>
          {typeof summary === "function" ? summary(state) : summary}
        </Text>
      </Pressable>
      {state ? children : null}
    </View>
  );
};

export { Details };
