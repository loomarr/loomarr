import type { Meta, StoryObj } from "@storybook/react-vite";
import { Switch } from "./switch";

// The pill toggle for "is this thing on?" — the v2 mock's source switches.
//
// Measured off the rendered mock: a 34×19px track, 1px border, 13px knob inset 2px travelling to
// 16px. ON is a `lock` tint with a solid `lock` knob; OFF is `background` with a `static-500`
// knob.
//
// ⚠ `lock` GREEN, not brand amber. A source being on is a *healthy* state, which is what `lock`
// means everywhere else in the app; amber is "this is what you are looking at". The control this
// replaced was an `accent-signal` checkbox — a small amber square where the mock has a green pill.
const meta = {
  title: "UI/Switch",
  component: Switch,
} satisfies Meta<typeof Switch>;

type Story = StoryObj<typeof meta>;

const On: Story = {
  args: { checked: true, "aria-label": "Use this source", onChange: () => {} },
};

const Off: Story = {
  args: { checked: false, "aria-label": "Use this source", onChange: () => {} },
};

// A switch the operator cannot flip right now — mid-request, or a row whose switch the server
// refuses. Dimmed rather than removed, so the control does not jump out of the layout.
const Disabled: Story = {
  args: { checked: true, disabled: true, "aria-label": "Use this source", onChange: () => {} },
};

export default meta;
export { Disabled, Off, On };
