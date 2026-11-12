import { Combobox, type ComboboxOption, Screen, Text } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-native";
import { useState } from "react";

const places: readonly ComboboxOption[] = [
  { description: "Washington, United States", label: "Seattle", value: "seattle" },
  { description: "Oregon, United States", label: "Portland", value: "portland" },
];

const NativeComboboxWorkshop = ({ density = "touch" }: { density?: "touch" | "tv" }) => {
  const [text, setText] = useState("");
  const needle = text.trim().toLowerCase();
  return (
    <Screen density={density} gap="section">
      <Text density={density} textRole="title">
        Combobox
      </Text>
      <Combobox
        density={density}
        emptyText="No locations found."
        inputValue={text}
        label="Location"
        onInputValueChange={setText}
        onSelect={(option) => setText(option.label)}
        options={needle ? places.filter((place) => place.label.toLowerCase().includes(needle)) : []}
        placeholder="Search for a city or country"
      />
    </Screen>
  );
};

const meta = {
  title: "Loomarr Foundations/Combobox",
  component: NativeComboboxWorkshop,
  args: { density: "touch" },
} satisfies Meta<typeof NativeComboboxWorkshop>;

type Story = StoryObj<typeof meta>;
const Touch: Story = {};
const Tv: Story = { args: { density: "tv" } };
const Light: Story = { globals: { theme: "light" } };

export default meta;
export { Light, Touch, Tv };
