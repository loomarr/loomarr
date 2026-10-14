import type { Meta, StoryObj } from "@storybook/react-vite";
import { TunerLoader } from "./tuner-loader";

// A bright, busy stand-in for the previous channel's held frame (sky, sun, hard horizon, stripes):
// the hard case #1620 fixed, where small amber text sat straight on the picture. A data: URI, so the
// story never reaches the network.
const HELD_FRAME = `data:image/svg+xml,${encodeURIComponent(
  `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 160 90" preserveAspectRatio="none">
    <defs>
      <linearGradient id="g" x1="0" y1="0" x2="0.35" y2="1">
        <stop offset="0" stop-color="#6fb7e8"/><stop offset="0.38" stop-color="#b9e0f5"/>
        <stop offset="0.39" stop-color="#f4d27a"/><stop offset="0.62" stop-color="#e89a4a"/>
        <stop offset="0.63" stop-color="#7a4a2a"/><stop offset="1" stop-color="#3f2a1c"/>
      </linearGradient>
      <pattern id="s" width="11" height="90" patternUnits="userSpaceOnUse">
        <rect width="3" height="90" fill="#fff" fill-opacity="0.08"/>
      </pattern>
    </defs>
    <rect width="160" height="90" fill="url(#g)"/>
    <circle cx="112" cy="27" r="14" fill="#ffe9a8"/>
    <rect width="160" height="90" fill="url(#s)"/>
  </svg>`,
)}`;

// The "acquiring signal" loader for the Watch player (§9.1, #1620 B4). It is absolute inset-0, so the
// story frames it in a relative 16:9 box on black — the player frame it overlays. Under the visual
// suite's reduced-motion pin the wash sits at its drained look, the snow is still (seeded, so the same
// every run) and the bars sit at their LOCKED amber frame, so the baseline is the settled readout.
const meta = {
  title: "Shell/TunerLoader",
  component: TunerLoader,
  decorators: [
    (Story) => (
      <div className="relative aspect-video w-[calc(100vw-2rem)] max-w-[720px] overflow-hidden rounded-xl bg-black">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof TunerLoader>;

type Story = StoryObj<typeof meta>;

// A cold start: nothing has played yet, so there is no picture under the loader to drain.
const Default: Story = { args: { channel: { number: 12, name: "Saturday Cartoons" } } };

// A channel switch over the brightest kind of held frame: the frame drains into the snow and the
// readout lands on static.
const Switch: Story = {
  args: { channel: { number: 12, name: "Saturday Cartoons" }, heldFrame: true },
  decorators: [
    (Story) => (
      <>
        <img src={HELD_FRAME} alt="" className="absolute inset-0 size-full object-cover" />
        <Story />
      </>
    ),
  ],
};

// A long channel name wraps inside the frame rather than running off it.
const LongName: Story = {
  args: { channel: { number: 108, name: "Saturday Morning Animation Marathon" }, heldFrame: true },
  decorators: Switch.decorators,
};

// A custom label — the slot the caller can rename (e.g. a different surface's warm-up copy).
const CustomLabel: Story = { args: { label: "ACQUIRING SIGNAL" } };

export default meta;
export { CustomLabel, Default, LongName, Switch };
