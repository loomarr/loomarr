import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "../button";
import { Card, CardContent, CardFooter, CardHeader, CardTitle } from "./card";

// Card — the surface everything else sits on. Its parts are separate components so a card's
// anatomy stays consistent wherever it appears; a hand-rolled bordered div is the drift.
const meta = {
  title: "Primitives/Card",
  component: Card,
} satisfies Meta<typeof Card>;

type Story = StoryObj<typeof meta>;

const Default: Story = {
  render: () => (
    <Card className="w-80">
      <CardHeader>
        <CardTitle>Springfield Classics</CardTitle>
      </CardHeader>
      <CardContent>
        <p className="text-muted-foreground text-sm">
          Channel 1 · 63 titles · on air. Plays what is ready and fills the rest.
        </p>
      </CardContent>
    </Card>
  ),
};

const WithFooter: Story = {
  render: () => (
    <Card className="w-80">
      <CardHeader>
        <CardTitle>Delete this channel?</CardTitle>
      </CardHeader>
      <CardContent>
        <p className="text-muted-foreground text-sm">
          The channel stops broadcasting immediately. Acquired titles stay in your library.
        </p>
      </CardContent>
      <CardFooter className="gap-2">
        <Button variant="destructive" size="sm">
          Delete
        </Button>
        <Button variant="outline" size="sm">
          Cancel
        </Button>
      </CardFooter>
    </Card>
  ),
};

export default meta;
export { Default, WithFooter };
