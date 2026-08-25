import type { FillerDecisionReviewsOutputBody } from "@loomarr/api";
import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { widthFrame, withRouter } from "@/test/story-utils";
import { FillerReviewQueue } from "./filler-review-queue";

const row = (index: number) => ({
  id: `decision-${index}`,
  clipHash: `abcdef0123456789${index}`,
  createdAt: new Date().toISOString(),
  question: index % 2 === 0 ? "Is this a soda commercial?" : "Is this a programme promo?",
  reasonCodes: ["brand_category_conflict"],
  evidenceRefs: ["transcript", "frame-2"],
  conflicts: [
    {
      claim: "product category",
      values: index % 2 === 0 ? ["Mountain Dew", "unknown"] : ["network promo", "commercial"],
      evidenceRefs: ["transcript", "frame-2"],
      resolved: false,
    },
  ],
});

const withReviews =
  (body: FillerDecisionReviewsOutputBody): Decorator =>
  (Story) => {
    window.fetch = (() =>
      Promise.resolve(
        new Response(JSON.stringify(body), { status: 200, headers: { "content-type": "application/json" } }),
      )) as typeof fetch;
    return (
      <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
        <Story />
      </QueryClientProvider>
    );
  };

const meta = {
  title: "Filler/DecisionReviewQueue",
  component: FillerReviewQueue,
  decorators: [widthFrame(760), withRouter("/filler/attention")],
} satisfies Meta<typeof FillerReviewQueue>;

export default meta;
type Story = StoryObj<typeof meta>;

export const GenuineReview: Story = { decorators: [withReviews({ rows: [row(1)], total: 1 })] };
export const Empty: Story = { decorators: [withReviews({ rows: [], total: 0 })] };
export const LargeQueue: Story = {
  decorators: [withReviews({ rows: Array.from({ length: 24 }, (_, index) => row(index)), total: 24 })],
};
export const Correction: Story = {
  decorators: [withReviews({ rows: [row(2)], total: 1 })],
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(await canvas.findByRole("button", { name: "Correct" }));
    await canvas.findByLabelText("Correction");
  },
};

const AnchoringPrototype = () => (
  <div className="space-y-4">
    <div>
      <h1 className="font-semibold text-xl">Review-ordering comparison</h1>
      <p className="text-muted-foreground text-sm">
        Prototype only. Production uses the evidence-first ordering on the left.
      </p>
    </div>
    <div className="grid gap-4 md:grid-cols-2">
      <Card className="p-5">
        <Badge variant="signal">Evidence first</Badge>
        <div className="mt-4 rounded-md border border-caution/35 bg-caution/5 p-3">
          <p className="font-medium text-sm">Conflicting product category</p>
          <p className="mt-1 text-muted-foreground text-sm">Mountain Dew · unknown</p>
        </div>
        <h2 className="mt-4 font-semibold text-lg">Is this a soda commercial?</h2>
        <div className="mt-4 flex gap-2">
          <Button>Accept</Button>
          <Button variant="outline">Correct</Button>
          <Button variant="ghost">Reject</Button>
        </div>
      </Card>
      <Card className="p-5">
        <Badge variant="caution">Proposal visible</Badge>
        <p className="mt-4 text-muted-foreground text-sm">Loomarr's proposed answer</p>
        <p className="font-semibold text-lg">Probably a soda commercial</p>
        <div className="mt-4 rounded-md border border-caution/35 bg-caution/5 p-3">
          <p className="font-medium text-sm">Conflicting product category</p>
          <p className="mt-1 text-muted-foreground text-sm">Mountain Dew · unknown</p>
        </div>
        <h2 className="mt-4 font-semibold text-lg">Is this a soda commercial?</h2>
        <div className="mt-4 flex gap-2">
          <Button>Accept</Button>
          <Button variant="outline">Correct</Button>
          <Button variant="ghost">Reject</Button>
        </div>
      </Card>
    </div>
  </div>
);

export const AnchoringComparison: Story = {
  render: () => <AnchoringPrototype />,
};
