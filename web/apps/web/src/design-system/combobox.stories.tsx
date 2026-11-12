import { Combobox, type ComboboxOption, Screen, Surface, Text } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";

// Combobox — a text field with a floating list of options (Base UI underneath). It does not filter
// and does not own the text: the caller narrows `options` (a debounced server search, or a local
// filter) and keeps `inputValue`. The first story is a local filter, the second a search.
const languages = [
  "Arabic",
  "Chinese",
  "Dutch",
  "English",
  "French",
  "German",
  "Hindi",
  "Italian",
  "Japanese",
  "Korean",
  "Portuguese",
  "Spanish",
].map((label): ComboboxOption => ({ label, value: label.toLowerCase() }));

const places: readonly ComboboxOption[] = [
  { description: "Washington, United States", label: "Seattle", value: "seattle" },
  { description: "Oregon, United States", label: "Portland", value: "portland" },
  { description: "England, United Kingdom", label: "Portsmouth", value: "portsmouth" },
  { description: "Ontario, Canada", label: "Port Colborne", trailing: "Added", value: "port-colborne" },
];

const LocalFilter = ({ disabled = false, invalid = false }: { disabled?: boolean; invalid?: boolean }) => {
  const [text, setText] = useState("");
  const [chosen, setChosen] = useState<string | null>("english");
  const needle = text.trim().toLowerCase();
  return (
    <Combobox
      autoHighlight
      disabled={disabled}
      emptyText="No languages found."
      invalid={invalid}
      inputValue={text}
      label="Commercial language"
      onInputValueChange={setText}
      onSelect={(option) => {
        setChosen(option.value);
        setText(option.label);
      }}
      options={needle ? languages.filter((option) => option.label.toLowerCase().includes(needle)) : languages}
      placeholder="Search languages"
      value={chosen}
    />
  );
};

const Search = ({
  error = false,
  loading = false,
  stale = false,
}: {
  error?: boolean;
  loading?: boolean;
  stale?: boolean;
}) => {
  const [text, setText] = useState("Port");
  const needle = text.trim().toLowerCase();
  const found = needle.length < 2 ? [] : places.filter((place) => place.label.toLowerCase().includes(needle));
  return (
    <Combobox
      emptyText="No locations found. Try a nearby city or country."
      errorText={error ? "Location search is unavailable. Try again." : undefined}
      inputValue={text}
      label="Location"
      loading={loading}
      onInputValueChange={setText}
      onSelect={(option) => setText(option.label)}
      options={error || loading ? [] : found}
      placeholder="Search for a city or country"
      stale={stale}
    />
  );
};

const Workshop = ({ disabled = false, invalid = false }: { disabled?: boolean; invalid?: boolean }) => (
  <Screen gap="section">
    <Surface gap="control" padding="section">
      <Text textRole="title">Local filter, first option highlighted</Text>
      <LocalFilter disabled={disabled} invalid={invalid} />
    </Surface>
    <Surface gap="control" padding="section">
      <Text textRole="title">Search with descriptions</Text>
      <Search />
    </Surface>
    <Surface gap="control" padding="section">
      <Text textRole="title">Searching, empty and failed</Text>
      <Search loading />
      <Search error />
    </Surface>
  </Screen>
);

const meta = {
  title: "Loomarr Components/Combobox",
  component: Workshop,
} satisfies Meta<typeof Workshop>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};
const Invalid: Story = { args: { invalid: true } };
const Disabled: Story = { args: { disabled: true } };
const Light: Story = { globals: { theme: "light" } };
// The list is open on arrival, so the baseline shows its surface, highlight, check and trailing text.
const Open: Story = {
  play: async ({ canvas, userEvent }) => {
    const [, search] = await canvas.findAllByRole("combobox");
    if (search) await userEvent.click(search);
  },
};

export default meta;
export { Default, Disabled, Invalid, Light, Open };
