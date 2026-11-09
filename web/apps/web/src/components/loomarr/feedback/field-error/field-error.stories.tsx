import { Field } from "@loomarr/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { FieldError } from "./field-error";

const meta = {
  title: "Feedback/FieldError",
  component: FieldError,
} satisfies Meta<typeof FieldError>;

type Story = StoryObj<typeof meta>;

// The password field as the app's forms draw it (lib/app-form's TextField).
const PasswordField = () => {
  const [value, setValue] = useState("short");
  return (
    <Field
      autoCapitalize="none"
      autoCorrect={false}
      id="story-password"
      invalid
      label="Password"
      onChangeText={setValue}
      secureTextEntry
      value={value}
    />
  );
};

// In context: the line under a field that failed validation on submit.
const UnderAField: Story = {
  render: (args) => (
    <div className="flex max-w-sm flex-col gap-2">
      <PasswordField />
      <FieldError {...args} />
    </div>
  ),
  args: { errors: [{ message: "Use at least 8 characters." }] },
};

export default meta;
export { UnderAField };
