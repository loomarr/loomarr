import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "../button";
import { Switch } from "../switch";
import { Disclosure } from "./disclosure";

// Disclosure — a reveal whose trigger is a discrete chevron rather than the whole header, so the
// row can hold its own controls. CollapsibleSection wraps its entire header in one <button>,
// which is invalid the moment anything else in the row is interactive.
const meta = {
  title: "Primitives/Disclosure",
  component: Disclosure,
  parameters: { layout: "padded" },
  args: { children: null },
} satisfies Meta<typeof Disclosure>;

type Story = StoryObj<typeof meta>;

// ⚠ THE case the primitive exists for: a row whose header carries a Switch and a Button beside
// the chevron. Wrapping this header in one <button> — what CollapsibleSection does — would nest
// interactive content inside a button, which is unreachable by keyboard and undefined in the
// accessibility tree.
const RowWithItsOwnControls: Story = {
  render: () => (
    <Disclosure className="rounded-lg border border-border">
      <div className="flex items-center gap-3 px-4 py-3">
        <Disclosure.Trigger label="Show Archive.org collections" />
        <span className="min-w-0 flex-1 truncate font-medium text-sm">Archive.org</span>
        <span className="text-static-400 text-xs">3 collections · 214 clips</span>
        <Switch aria-label="Enable Archive.org" defaultChecked />
        <Button size="sm" variant="ghost">
          Scan
        </Button>
      </div>
      <Disclosure.Panel className="border-border border-t px-4 py-3 text-muted-foreground text-sm">
        Three collection rows would sit here, each with its own leaf controls.
      </Disclosure.Panel>
    </Disclosure>
  ),
};

// Several open at once — deliberately NOT an Accordion. A queue of clips or a tree of sources
// must be comparable side by side, so nothing enforces single-open exclusivity.
const SeveralOpenAtOnce: Story = {
  render: () => (
    <div className="flex flex-col gap-2">
      {["Coca-Cola 1985", "Pepsi 1984", "Fanta 1991"].map((name) => (
        <Disclosure key={name} defaultOpen className="rounded-lg border border-border">
          <div className="flex items-center gap-3 px-4 py-3">
            <Disclosure.Trigger label={`Show what happened to ${name}`} />
            <span className="min-w-0 flex-1 truncate font-medium text-sm">{name}</span>
          </div>
          <Disclosure.Panel className="border-border border-t px-4 py-3 text-muted-foreground text-sm">
            The stage-by-stage detail for {name}.
          </Disclosure.Panel>
        </Disclosure>
      ))}
    </div>
  ),
};

export default meta;
export { RowWithItsOwnControls, SeveralOpenAtOnce };
