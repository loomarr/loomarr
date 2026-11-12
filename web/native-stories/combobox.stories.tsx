import { Combobox, type ComboboxGroup, type ComboboxOption, Screen, Text } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-native";
import { useState } from "react";

const places: readonly ComboboxOption[] = [
  { description: "Washington, United States", label: "Seattle", value: "seattle" },
  { description: "Oregon, United States", label: "Portland", value: "portland" },
];

const grouped: readonly ComboboxGroup[] = [
  { id: "channels", label: "Channels", options: [{ label: "90s Action", trailing: "#42", value: "c1" }] },
  {
    id: "library",
    label: "In your library",
    options: [
      { label: "The Velvet Cartographer", trailing: "1995", value: "l1" },
      { label: "Pelican Point Motel", trailing: "1997", value: "l2" },
    ],
  },
];

// Grouped and always open: the shape the command palette takes on a phone or a TV.
const GroupedInline = ({ density }: { density: "touch" | "tv" }) => {
  const [text, setText] = useState("a");
  return (
    <Combobox
      density={density}
      emptyText="No matches."
      groups={grouped}
      inline
      inputValue={text}
      label="Search"
      onInputValueChange={setText}
      onSelect={(option) => setText(option.label)}
      placeholder="Search titles and channels"
    />
  );
};

const NativeComboboxWorkshop = ({ density = "touch" }: { density?: "touch" | "tv" }) => {
  const [text, setText] = useState("");
  const needle = text.trim().toLowerCase();
  return (
    <Screen density={density} gap="section">
      <Text density={density} textRole="title">
        Combobox
      </Text>
      <GroupedInline density={density} />
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
