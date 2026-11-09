import { ChoiceCard, Icon, icons, Screen, Surface, Text } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";

// The selectable card (#2147): the starter templates and the library facets a reader picks from.
const templates = [
  { description: "Cartoons and a kids' movie to start the weekend.", id: "sat", title: "Saturday morning" },
  { description: "The sitcoms you grew up with, in one lineup.", id: "sit", title: "90s sitcoms" },
  { description: "Scary movies leading up to the 31st.", id: "hor", title: "Horror for Halloween" },
] as const;

const Templates = () => {
  const [chosen, setChosen] = useState<string>("sit");
  return (
    <Surface backgroundColor="transparent" borderWidth={0} gap="inline">
      {templates.map((template) => (
        <ChoiceCard
          description={template.description}
          key={template.id}
          leading={<Icon decorative glyph={icons.suggest} size="compact" tone="suggest" />}
          onPress={() => setChosen(template.id)}
          selected={chosen === template.id}
          title={template.title}
        />
      ))}
    </Surface>
  );
};

const ChoiceCardWorkshop = () => (
  <Screen gap="section">
    <Surface gap="control" padding="section">
      <Text textRole="title">Starter templates</Text>
      <Templates />
    </Surface>
    <Surface gap="control" padding="section">
      <Text textRole="title">Library facets and a disabled card</Text>
      <Surface backgroundColor="transparent" borderWidth={0} flexDirection="row" flexWrap="wrap" gap="inline">
        <ChoiceCard accent="info" description="42 titles" onPress={() => undefined} selected title="Comedy" />
        <ChoiceCard
          accent="info"
          description="18 titles"
          onPress={() => undefined}
          selected={false}
          title="Animation"
        />
        <ChoiceCard disabled onPress={() => undefined} title="Starting…" />
      </Surface>
    </Surface>
  </Screen>
);

const meta = {
  title: "Loomarr Components/ChoiceCard",
  component: ChoiceCardWorkshop,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof ChoiceCardWorkshop>;

type Story = StoryObj<typeof meta>;
const Workshop: Story = {};

export default meta;
export { Workshop };
