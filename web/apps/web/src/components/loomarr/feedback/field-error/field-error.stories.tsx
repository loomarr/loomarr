import type { Meta, StoryObj } from "@storybook/react-vite";
import { Input, Label } from "@/components/ui";
import { FieldError } from "./field-error";

const meta = {
  title: "Feedback/FieldError",
  component: FieldError,
} satisfies Meta<typeof FieldError>;

type Story = StoryObj<typeof meta>;

// In context: the line under a field that failed validation on submit.
const UnderAField: Story = {
  render: (args) => (
    <div className="flex max-w-sm flex-col gap-2">
      <Label htmlFor="story-password">Password</Label>
      <Input id="story-password" type="password" aria-invalid="true" defaultValue="short" />
      <FieldError {...args} />
    </div>
  ),
  args: { errors: [{ message: "Use at least 8 characters." }] },
};

export default meta;
export { UnderAField };
