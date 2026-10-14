import { semanticColors } from "@loomarr/design-system";
import {
  type OnScreenLogoHoliday,
  type OnScreenLogoPolicy,
  type OnScreenLogoPreviewRequest,
  OnScreenLogoSection,
} from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";

// The backend draws the real PNG; a story stands in with an inline SVG that names what was asked
// for (no remote images in stories), so each state's preview is still distinguishable.
const previewSrc = (r: OnScreenLogoPreviewRequest) => {
  if (r.motifOnly) {
    const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="48" height="48"><circle cx="24" cy="24" r="20" fill="${semanticColors.state.info}"/></svg>`;
    return `data:image/svg+xml;utf8,${encodeURIComponent(svg)}`;
  }
  const label = `SC ${r.look ?? "text"}${r.holiday ? ` + ${r.picture ?? r.holiday}` : ""}`;
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="240" height="64"><rect x="2" y="8" width="236" height="48" rx="6" fill="${semanticColors.state.info}"/><text x="120" y="40" font-family="sans-serif" font-size="20" font-weight="700" text-anchor="middle" fill="${semanticColors.content.primary}">${label}</text></svg>`;
  return `data:image/svg+xml;utf8,${encodeURIComponent(svg)}`;
};

const holidays = (activeId?: string): OnScreenLogoHoliday[] => [
  {
    id: "halloween",
    name: "Halloween",
    start: "2026-10-01",
    end: "2026-10-31",
    active: activeId === "halloween",
    picture: "pumpkin",
    pictures: [
      { id: "pumpkin", name: "Pumpkin" },
      { id: "bat", name: "Bat" },
      { id: "ghost", name: "Ghost" },
    ],
  },
  {
    id: "christmas",
    name: "Christmas",
    start: "2026-12-01",
    end: "2026-12-26",
    active: false,
    picture: "holly",
    pictures: [
      { id: "holly", name: "Holly" },
      { id: "snowflake", name: "Snowflake" },
    ],
  },
  {
    id: "valentines",
    name: "Valentine’s Day",
    start: "2027-02-01",
    end: "2027-02-14",
    active: false,
    picture: "heart",
    pictures: [{ id: "heart", name: "Heart" }],
  },
];

const noop = () => undefined;

const meta = {
  title: "Loomarr Components/On-screen Logo",
  component: OnScreenLogoSection,
  args: {
    autoCallsign: "SC",
    channelNumber: 7,
    defaults: { look: "text", opacity: 0.4 },
    drawable: true,
    eraLabel: "1980s",
    everyHoliday: false,
    holidays: holidays(),
    onChange: noop,
    onOpenProgramming: noop,
    onRemoveImage: noop,
    onUploadImage: noop,
    policy: { look: "tint" },
    previewSrc,
    today: new Date(2026, 6, 4),
  },
  decorators: [
    (Story) => (
      <div style={{ boxSizing: "border-box", minHeight: "100vh", padding: 32 }}>
        <Story />
      </div>
    ),
  ],
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof OnScreenLogoSection>;

type Story = StoryObj<typeof meta>;

const Normal: Story = {};
const HolidayPreview: Story = {
  args: { holidays: holidays("halloween"), today: new Date(2026, 9, 13) },
};
const IgnoreHolidays: Story = { args: { holidays: [] } };
const OwnImage: Story = { args: { policy: { image: "abc123" } } };
const CantDraw: Story = { args: { drawable: false } };
const NoEra: Story = { args: { eraLabel: undefined, everyHoliday: true } };

const style = (look: NonNullable<OnScreenLogoPolicy["look"]>): Story => ({ args: { policy: { look } } });
const StylePlate = style("plate");
const StyleText = style("text");
const StyleOutline = style("outline");
const StyleSmallPlate = style("small-plate");
const StyleTint = style("tint");
const StyleTwoTone = style("twotone");
const StyleBadge = style("badge");
const StyleLockup = style("lockup");
const StyleGlass = style("glass");
const StyleStripe = style("stripe");

export default meta;
export {
  CantDraw,
  HolidayPreview,
  IgnoreHolidays,
  NoEra,
  Normal,
  OwnImage,
  StyleBadge,
  StyleGlass,
  StyleLockup,
  StyleOutline,
  StylePlate,
  StyleSmallPlate,
  StyleStripe,
  StyleText,
  StyleTint,
  StyleTwoTone,
};
