import type { Decorator } from "@storybook/react-vite";

// A fixed-width frame so `w-full` components (IntentInput, ProposalReview, SearchCommand,
// …) render at a realistic size inside the centered story canvas — the snapshot then
// bounds the component the way it sits in a real page column, not collapsed to zero.
const widthFrame =
  (px: number): Decorator =>
  (Story) => (
    <div style={{ width: px, maxWidth: "100%" }}>
      <Story />
    </div>
  );

export { widthFrame };
