import type { Meta, StoryObj } from "@storybook/react-vite";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle, SheetTrigger } from "./sheet";

const meta = {
  title: "UI/Sheet",
  component: Sheet,
  render: () => (
    <Sheet swipeDirection="right">
      <SheetTrigger className="rounded-md border border-input px-4 py-2">Open sheet</SheetTrigger>
      <SheetContent>
        <SheetHeader>
          <SheetTitle>Person details</SheetTitle>
          <SheetDescription>Manage access and sessions.</SheetDescription>
        </SheetHeader>
      </SheetContent>
    </Sheet>
  ),
} satisfies Meta<typeof Sheet>;

type Story = StoryObj<typeof meta>;
const Default: Story = {};

export default meta;
export { Default };
