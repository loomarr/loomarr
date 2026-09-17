import { getListFillerSourcesQueryKey } from "@loomarr/api/endpoints/filler";
import { getSettingsListQueryKey } from "@loomarr/api/endpoints/settings";
import { fillerRefinementSettings } from "@loomarr/fixtures";
import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { SettingsEditsProvider } from "@/settings/settings-edits";
import { widthFrame, withRouter } from "@/test/story-utils";
import { FillerSettings } from "./filler-settings";

const withSettings: Decorator = (Story) => {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
  client.setQueryData(getSettingsListQueryKey(), {
    status: 200,
    data: fillerRefinementSettings,
    headers: new Headers(),
  });
  client.setQueryData(getListFillerSourcesQueryKey(), {
    status: 200,
    data: { sources: [], total: 0 },
    headers: new Headers(),
  });
  return (
    <QueryClientProvider client={client}>
      <SettingsEditsProvider>
        <Story />
      </SettingsEditsProvider>
    </QueryClientProvider>
  );
};
const meta = {
  title: "Filler/Settings",
  component: FillerSettings,
  decorators: [widthFrame(760), withSettings],
} satisfies Meta<typeof FillerSettings>;
type Story = StoryObj<typeof meta>;

const Downloads: Story = { decorators: [withRouter("/filler")] };
const Storage: Story = {
  args: { section: "storage" },
  decorators: [withRouter("/filler")],
};

export default meta;
export { Downloads, Storage };
