import { Chip, Screen, Surface, Text } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";

// The toggle chip (#2147): the Guide's span picker, a lineup's sort, a suggestion to tap.
const SpanPicker = () => {
  const [span, setSpan] = useState("2H");
  return (
    <Surface
      alignItems="center"
      backgroundColor="transparent"
      borderWidth={0}
      flexDirection="row"
      gap="inline"
    >
      <Text shout textRole="captionMono">
        Span
      </Text>
      {["1H", "2H", "4H", "8H"].map((label) => (
        <Chip key={label} mono onPress={() => setSpan(label)} selected={span === label}>
          {label}
        </Chip>
      ))}
    </Surface>
  );
};

const ChipWorkshop = () => (
  <Screen gap="section">
    <Surface gap="control" padding="section">
      <Text textRole="title">Toggle chips</Text>
      <SpanPicker />
    </Surface>
    <Surface gap="control" padding="section">
      <Text textRole="title">Action chip and states</Text>
      <Surface
        alignItems="center"
        backgroundColor="transparent"
        borderWidth={0}
        flexDirection="row"
        gap="inline"
      >
        <Chip mono onPress={() => undefined} tone="info">
          SCHEDULED · back to today
        </Chip>
        <Chip disabled mono onPress={() => undefined} selected={false}>
          Year
        </Chip>
        <Chip mono onPress={() => undefined} selected>
          Play order
        </Chip>
        <Chip onPress={() => undefined} selected={false} shape="pill" tone="danger">
          2 errors
        </Chip>
        <Chip onPress={() => undefined} selected shape="pill" tone="warning">
          5 warnings
        </Chip>
      </Surface>
    </Surface>
    <Surface gap="control" padding="section">
      <Text textRole="title">Pills to fill a field</Text>
      <Surface backgroundColor="transparent" borderWidth={0} flexDirection="row" flexWrap="wrap" gap="inline">
        {["Saturday morning cartoons", "90s sitcoms", "Horror for Halloween"].map((label) => (
          <Chip key={label} onPress={() => undefined} shape="pill">
            {label}
          </Chip>
        ))}
      </Surface>
    </Surface>
  </Screen>
);

const meta = {
  title: "Loomarr Components/Chip",
  component: ChipWorkshop,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof ChipWorkshop>;

type Story = StoryObj<typeof meta>;
const Workshop: Story = {};

export default meta;
export { Workshop };
