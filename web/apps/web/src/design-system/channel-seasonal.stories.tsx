import {
  ChannelSeasonalFields,
  type SeasonalHoliday,
  type SeasonalMode,
  type SeasonalOffSeason,
  type SeasonalStrength,
} from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";

const HOLIDAYS: readonly SeasonalHoliday[] = [
  { id: "christmas", label: "Christmas" },
  { id: "halloween", label: "Halloween" },
  { id: "thanksgiving", label: "Thanksgiving" },
  { id: "newyear", label: "New Year" },
];

type Seed = {
  favourEvenings?: boolean;
  included?: readonly string[];
  mode?: SeasonalMode;
  offSeason?: SeasonalOffSeason;
  strength?: SeasonalStrength;
  strengthConsequence?: string;
};

// The fields are controlled; the story owns the state so every choice can be clicked through.
const SeasonalWorkshop = ({
  favourEvenings: initialEvenings = true,
  included,
  mode: initialMode = "default",
  offSeason: initialOffSeason = "default",
  strength: initialStrength = "default",
  strengthConsequence,
}: Seed) => {
  const [mode, setMode] = useState(initialMode);
  const [strength, setStrength] = useState(initialStrength);
  const [favourEvenings, setFavourEvenings] = useState(initialEvenings);
  const [offSeason, setOffSeason] = useState(initialOffSeason);
  const [picked, setPicked] = useState<readonly string[]>(included ?? HOLIDAYS.map((h) => h.id));
  return (
    <div style={{ boxSizing: "border-box", maxWidth: 560, padding: 24 }}>
      <ChannelSeasonalFields
        allHolidaysIncluded={picked.length === HOLIDAYS.length}
        favourEvenings={favourEvenings}
        holidays={HOLIDAYS}
        includedHolidayIds={picked}
        mode={mode}
        offSeason={offSeason}
        onFavourEveningsChange={setFavourEvenings}
        onHolidayToggle={(id, on) =>
          setPicked(HOLIDAYS.map((h) => h.id).filter((hid) => (hid === id ? on : picked.includes(hid))))
        }
        onModeChange={setMode}
        onOffSeasonChange={setOffSeason}
        onStrengthChange={setStrength}
        strength={strength}
        strengthConsequence={strengthConsequence}
      />
    </div>
  );
};

const meta = {
  title: "Loomarr Components/Channel Seasonal",
  component: SeasonalWorkshop,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof SeasonalWorkshop>;

type Story = StoryObj<typeof meta>;

// Built-in rotation, nothing chosen: Featured, evenings on, every holiday included.
const Default: Story = {};

// Built-in rotation in October with the backend's count for the open holiday.
const HalloweenOn: Story = {
  args: {
    strengthConsequence:
      "Halloween is on now. 9 Halloween episodes found: 3 a day, each back about every 3 days.",
  },
};

const Light: Story = { args: { strength: "light" } };

// Heavy on a small pool: nine episodes can only fill four a day without a repeat inside two days.
const HeavyAnytime: Story = {
  args: {
    favourEvenings: false,
    strength: "heavy",
    strengthConsequence:
      "Halloween is on now. 9 Halloween episodes found: 4 a day, each back about every 2 days.",
  },
};

const NoEpisodes: Story = {
  args: { strengthConsequence: "Next up: Christmas. No Christmas episodes found in this lineup." },
};

const PickedHolidays: Story = { args: { included: ["halloween", "christmas"] } };

// Holidays ignored: the holiday list, strength and evenings are all hidden because nothing reads them.
const Off: Story = { args: { mode: "off" } };

// Holiday-only is the one mode that consults the out-of-season fallback.
const HolidayChannel: Story = { args: { included: ["christmas"], mode: "exclusive", offSeason: "dark" } };

export default meta;
export { Default, HalloweenOn, HeavyAnytime, HolidayChannel, Light, NoEpisodes, Off, PickedHolidays };
