import type { Meta, StoryObj } from "@storybook/react-vite";
import { widthFrame, withRouter } from "@/test/story-utils";
import { RequestCard } from "./request-card";

// One request on the Requests page. The whole card links to the request's detail. ⚠ withRouter
// because the card is a TanStack Link, which needs a RouterProvider.
const meta = {
  title: "AI/RequestCard",
  component: RequestCard,
  decorators: [widthFrame(560), withRouter("/guide")],
  args: {
    jobId: "job-1",
    title: "Saturday morning cartoons for the kids",
    createdAt: "2026-09-24T18:00:00Z",
  },
} satisfies Meta<typeof RequestCard>;

type Story = StoryObj<typeof meta>;

const Generating: Story = { args: { line: "Generating", tone: "suggest" } };

// The In progress card names the live stage and pick count from the streamed progress.
const GeneratingLive: Story = {
  args: { line: "Generating", tone: "suggest", hint: "Choosing titles… · 4 of about 8 picked" },
};

const GettingTitles: Story = {
  args: { line: "Getting 3 titles (2 downloading, 1 waiting)", tone: "caution" },
};

const CouldntGet: Story = { args: { line: "Couldn't get 2 titles", tone: "onair" } };

const OnYourChannel: Story = { args: { line: "On your channel", tone: "lock" } };

// A failed request on Needs you: the reason, what to do about it, and the ONE fix action beside
// the card (outside the detail link).
const Failed: Story = {
  args: {
    line: "Couldn't build",
    tone: "onair",
    hint: "The model took too long. Try again in a moment.",
    action: (
      <a href="/guide" className="font-medium text-sm underline">
        Try again
      </a>
    ),
  },
};

export default meta;
export { CouldntGet, Failed, Generating, GeneratingLive, GettingTitles, OnYourChannel };
