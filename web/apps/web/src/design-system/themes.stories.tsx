import {
  Badge,
  FocusSurface,
  Icon,
  icons,
  LoomarrProvider,
  ProgressTrack,
  Screen,
  Surface,
  semanticThemes,
  Text,
} from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";

const ThemePanel = ({ mode }: { mode: "dark" | "light" }) => (
  <LoomarrProvider theme={mode}>
    <Screen density="pointer" gap="$section">
      <Surface backgroundColor="$transparent" borderWidth={0} gap={4}>
        <Text textRole="metadata">{mode.toUpperCase()} THEME</Text>
        <Text textRole="display">Broadcast clarity, any time.</Text>
        <Text textRole="body">
          The chroma palette stays Loomarr. Semantic surfaces and content adapt to the viewing environment.
        </Text>
      </Surface>

      <Surface gap="$control" padding="$section">
        <Text textRole="title">Ordinary surface</Text>
        <Text textRole="body">
          Primary copy carries the programme. Secondary copy recedes without becoming illegible.
        </Text>
        <Text textRole="metadata">CHANNEL 07 · 7:00–7:30 PM · S07E02</Text>
        <ProgressTrack accessibilityLabel="Programme progress" percent={62} width="100%" />
      </Surface>

      <FocusSurface focused gap="$control" padding="$section">
        <Surface
          alignItems="center"
          backgroundColor="$transparent"
          borderWidth={0}
          flexDirection="row"
          gap="$inline"
        >
          <Icon accessibilityLabel="Play" glyph={icons.play} tone="primary" />
          <Text textRole="title">Focused action</Text>
        </Surface>
        <Text textRole="body">
          Focus is a semantic state with sufficient non-text contrast in either mode.
        </Text>
      </FocusSurface>

      <Surface
        backgroundColor="$transparent"
        borderWidth={0}
        flexDirection="row"
        flexWrap="wrap"
        gap="$inline"
      >
        <Badge tone="live">ON NOW</Badge>
        <Badge tone="success">SIGNAL LOCKED</Badge>
        <Badge tone="warning">SCHEDULE DRIFT</Badge>
        <Badge tone="info">TUNING</Badge>
      </Surface>
    </Screen>
  </LoomarrProvider>
);

const ThemeGallery = () => (
  <div
    style={{
      background: semanticThemes.dark.surface.canvas,
      display: "grid",
      gridTemplateColumns: "repeat(2, minmax(0, 1fr))",
      minHeight: "100vh",
    }}
  >
    <ThemePanel mode="dark" />
    <ThemePanel mode="light" />
  </div>
);

const meta = {
  title: "Loomarr Foundations/Themes",
  component: ThemeGallery,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof ThemeGallery>;

type Story = StoryObj<typeof meta>;
const LightAndDark: Story = {};

export default meta;
export { LightAndDark };
