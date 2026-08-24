import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame } from "@/test/story-utils";
import { DiagnosticsPrototype } from "./diagnostics-prototype";

const meta = {
  title: "Diagnostics/Prototype/LogsFirst",
  component: DiagnosticsPrototype,
  decorators: [widthFrame(1200)],
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof DiagnosticsPrototype>;

type Story = StoryObj<typeof meta>;

const LiveStream: Story = { args: { variant: "A" } };
const IncidentTriage: Story = { args: { variant: "B" } };
const ReadableTimeline: Story = { args: { variant: "C" } };

export default meta;
export { IncidentTriage, LiveStream, ReadableTimeline };
