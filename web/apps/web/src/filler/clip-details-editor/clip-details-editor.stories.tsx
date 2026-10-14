import { getListTaxonomyQueryKey } from "@loomarr/api/endpoints/filler";
import {
  fillerRefinementKnownClip,
  fillerRefinementTaxonomy,
  fillerRefinementUnknownClip,
} from "@loomarr/fixtures";
import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { widthFrame } from "@/test/story-utils";
import { ClipDetailsEditor } from "./clip-details-editor";

const withVocabulary: Decorator = (Story) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  client.setQueryData(getListTaxonomyQueryKey(), {
    status: 200,
    data: fillerRefinementTaxonomy,
    headers: new Headers(),
  });
  return (
    <QueryClientProvider client={client}>
      <Story />
    </QueryClientProvider>
  );
};
const meta = {
  title: "Filler/ClipDetailsEditor",
  component: ClipDetailsEditor,
  decorators: [widthFrame(560), withVocabulary],
  args: { onClose: () => {} },
} satisfies Meta<typeof ClipDetailsEditor>;
type Story = StoryObj<typeof meta>;

const Known: Story = { args: { clip: fillerRefinementKnownClip } };
const Unknown: Story = { args: { clip: fillerRefinementUnknownClip } };

export default meta;
export { Known, Unknown };
