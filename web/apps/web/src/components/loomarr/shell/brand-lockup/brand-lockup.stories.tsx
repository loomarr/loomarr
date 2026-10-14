import type { Meta, StoryObj } from "@storybook/react-vite";
import { BrandLockup } from "./brand-lockup";

// The LOOMARR mark (§1): `hero` for login/wizard, `compact` for the sidebar. Pins the
// wordmark type + the strip together so a drift in either fails the gallery.
const meta = {
  title: "Shell/BrandLockup",
  component: BrandLockup,
} satisfies Meta<typeof BrandLockup>;

type Story = StoryObj<typeof meta>;

const Hero: Story = { args: { variant: "hero" } };
const HeroNoTagline: Story = { args: { variant: "hero", tagline: false } };
const Compact: Story = { args: { variant: "compact" } };

export default meta;
export { Compact, Hero, HeroNoTagline };
