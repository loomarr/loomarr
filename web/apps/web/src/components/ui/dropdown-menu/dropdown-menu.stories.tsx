import type { Meta, StoryObj } from "@storybook/react-vite";
import { Volume2 } from "lucide-react";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "./dropdown-menu";

const meta = {
  title: "UI/DropdownMenu",
  component: DropdownMenu,
} satisfies Meta<typeof DropdownMenu>;

export default meta;
type Story = StoryObj<typeof meta>;

// The single-select pattern the player's audio/subtitle controls use: an icon trigger + a menu of
// checkbox items where exactly one is checked.
const SingleSelect = () => {
  const [value, setValue] = useState("eng");
  const opts = [
    { v: "eng", label: "English · stereo" },
    { v: "spa", label: "Spanish · 5.1" },
    { v: "fra", label: "French" },
  ];
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon" aria-label="Audio">
          <Volume2 aria-hidden />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuLabel>Audio</DropdownMenuLabel>
        {opts.map((o) => (
          <DropdownMenuCheckboxItem key={o.v} checked={value === o.v} onCheckedChange={() => setValue(o.v)}>
            {o.label}
          </DropdownMenuCheckboxItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
};

export const IconSingleSelect: Story = { render: () => <SingleSelect /> };
