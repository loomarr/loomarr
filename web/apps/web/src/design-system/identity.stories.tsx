import {
  ChannelIdentity,
  type ChannelIdentityData,
  ProgrammeIdentity,
  type ProgrammeIdentityData,
} from "@loomarr/ui";
import { type Density, Screen, Surface, Text } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";

type IdentityVariant = "long" | "minimal" | "missing-logo" | "representative";

const representativeChannel: ChannelIdentityData = {
  channelLogoState: "ready",
  channelName: "Classic Animation",
  channelNumber: "07",
};

const representativeProgramme: ProgrammeIdentityData = {
  badge: { label: "On now", tone: "live" },
  description:
    "A fast-talking stranger convinces Springfield to build a monorail with a suspicious windfall.",
  episodeLabel: "S04E12",
  seriesTitle: "The Simpsons",
  timeLabel: "7:00–7:30 PM",
  title: "Marge vs. the Monorail",
};

const Logo = () => (
  <div
    style={{
      alignItems: "center",
      background: "linear-gradient(145deg, #4CC9E8, #FFB020)",
      color: "#0B0C0E",
      display: "flex",
      fontSize: 18,
      fontWeight: 800,
      height: "100%",
      justifyContent: "center",
      width: "100%",
    }}
  >
    CA
  </div>
);

const IdentityWorkshop = ({
  density = "pointer",
  variant = "representative",
}: {
  density?: Density;
  variant?: IdentityVariant;
}) => {
  const channel: ChannelIdentityData = {
    ...representativeChannel,
    channelLogoState: variant === "missing-logo" ? "missing" : "ready",
    channelName:
      variant === "long" ? "Springfield Classic Animation and Community Television" : "Classic Animation",
  };
  const programme: ProgrammeIdentityData =
    variant === "minimal"
      ? { timeLabel: "7:30–8:00 PM", title: "Untitled programme" }
      : {
          ...representativeProgramme,
          description:
            variant === "long"
              ? "Springfield considers an ambitious transit proposal while the family investigates the stranger behind a promise that sounds much too good to be true."
              : representativeProgramme.description,
          title:
            variant === "long"
              ? "A Very Long Programme Title That Must Remain Legible Without Breaking the Client Layout"
              : representativeProgramme.title,
        };

  return (
    <Screen density={density} gap="$section">
      <Text density={density} textRole="metadata" tone="info">
        AUTHORITATIVE MEDIA IDENTITY
      </Text>
      <Surface gap="$section" maxWidth={density === "tv" ? 960 : 680} padding="$section" width="100%">
        <ChannelIdentity
          channel={channel}
          density={density}
          logo={channel.channelLogoState === "ready" ? <Logo /> : undefined}
        />
        <ProgrammeIdentity density={density} programme={programme} />
      </Surface>
    </Screen>
  );
};

const meta = {
  title: "Loomarr Components/Media Identity",
  component: IdentityWorkshop,
  args: { density: "pointer", variant: "representative" },
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof IdentityWorkshop>;

type Story = StoryObj<typeof meta>;
const Representative: Story = {};
const MissingLogo: Story = { args: { variant: "missing-logo" } };
const MinimalMetadata: Story = { args: { variant: "minimal" } };
const LongContent: Story = { args: { variant: "long" } };
const Touch: Story = { args: { density: "touch" } };
const Tv: Story = { args: { density: "tv" } };
const Light: Story = { globals: { theme: "light" } };

export default meta;
export { Light, LongContent, MinimalMetadata, MissingLogo, Representative, Touch, Tv };
