import {
  Combobox,
  type ComboboxGroup,
  type ComboboxOption,
  Screen,
  Surface,
  Text,
} from "@loomarr/design-system";
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

const found: readonly ComboboxGroup[] = [
  { id: "channels", label: "Channels", options: [{ label: "90s Action", trailing: "#42", value: "c1" }] },
  {
    id: "library",
    label: "In your library",
    options: [
      { label: "The Velvet Cartographer", trailing: "1995", value: "l1" },
      { label: "Pelican Point Motel", description: "A motel at the end of the line", value: "l2" },
    ],
  },
  { id: "tmdb", label: "Discover", options: [{ label: "Harbour Lights", trailing: "1997", value: "t1" }] },
];

// Inline and grouped: always open, no popup, the panel carrying the header (a leading mark, a key
// hint) around a bare field. The command palette's shape; Home and End jump to the ends.
const InlinePalette = ({ empty = false, fill = false }: { empty?: boolean; fill?: boolean }) => {
  const [text, setText] = useState(empty ? "zzz" : "a");
  return (
    <div style={{ height: fill ? 420 : undefined, width: 560 }}>
      <Combobox
        autoHighlight="always"
        emptyText={text ? `No matches for “${text}”.` : "Type to search."}
        endAdornment={
          <Text density="pointer" textRole="metadata" tone="secondary">
            Esc
          </Text>
        }
        fill={fill}
        groups={empty ? [] : found}
        inline
        inputValue={text}
        label="Search"
        listLabel="Search results"
        onInputValueChange={setText}
        onSelect={(option) => setText(option.label)}
        placeholder="Search titles, channels, help…"
      />
    </div>
  );
};

const meta = {
  title: "Loomarr Components/Combobox",
  component: Workshop,
} satisfies Meta<typeof Workshop>;

type Story = StoryObj<typeof meta>;

const Default: Story = {};
const Inline: Story = { render: () => <InlinePalette /> };
const InlineEmpty: Story = { render: () => <InlinePalette empty /> };
const InlineFill: Story = { render: () => <InlinePalette fill /> };
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
export { Default, Disabled, Inline, InlineEmpty, InlineFill, Invalid, Light, Open };
