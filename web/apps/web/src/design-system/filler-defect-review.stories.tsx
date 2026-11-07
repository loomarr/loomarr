import type { FillerDefectReportDTO } from "@loomarr/core/filler-defects";
import { DefectReview } from "@loomarr/ui";
import type { Meta, StoryObj } from "@storybook/react-vite";

// The host's clip player stands in as a plain 4:3 frame: stories load no remote media.
const player = <div style={{ aspectRatio: "4 / 3", background: "#111", width: "100%" }} />;

const report: FillerDefectReportDTO = {
  floorVersion: "floor-v3-provisional",
  clips: 210,
  unmeasured: 3,
  labelled: 42,
  verdicts: { pass: 180, fail: 23, undecided: 0, pending: 4 },
  scores: [
    {
      band: "tape",
      precision: 1,
      caught: 0.83,
      failed: 5,
      falseFails: 0,
      judged: 0,
      missed: 1,
      borderlinePass: 2,
      judgeRight: 3,
      judgeWrong: 1,
      total: 12,
    },
    {
      band: "sd",
      precision: 0.95,
      caught: 0.9,
      failed: 19,
      falseFails: 1,
      judged: 8,
      missed: 3,
      borderlinePass: 4,
      judgeRight: 5,
      judgeWrong: 1,
      total: 30,
    },
  ],
  total: 41,
  rows: [
    {
      clipId: "c1",
      name: "Sunbeam Juice",
      era: 1988,
      band: "tape",
      outcome: "borderline",
      verdict: "pass",
      verdictReason: "judge",
      findings: [
        { check: "blocks", outcome: "pass", measured: 1.1, bar: 4 },
        { check: "blur", outcome: "borderline", measured: 8.7, bar: 8 },
        { check: "dropouts", outcome: "borderline", measured: 0.0018, bar: 0.0015 },
      ],
      judgement: { ruling: "pass", reason: "soft but clean VHS", model: "gemma3" },
      measuredAt: "2026-10-27T12:00:00Z",
    },
  ],
};

const meta = {
  title: "Loomarr Components/Filler Defect Review",
  component: DefectReview,
  decorators: [
    (Story) => (
      <div style={{ boxSizing: "border-box", maxWidth: 960, minHeight: "100vh", padding: 48 }}>
        <Story />
      </div>
    ),
  ],
  args: { onLabel: () => undefined, onSkip: () => undefined, player, report },
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof DefectReview>;

type Story = StoryObj<typeof meta>;

const Queue: Story = {};
const NothingLabelledYet: Story = { args: { report: { ...report, labelled: 0, scores: [] } } };
const AllLabelled: Story = { args: { report: { ...report, rows: [], total: 0 } } };
const SaveFailed: Story = { args: { error: "The server couldn't be reached." } };
const Light: Story = { globals: { theme: "light" } };

export default meta;
export { AllLabelled, Light, NothingLabelledYet, Queue, SaveFailed };
