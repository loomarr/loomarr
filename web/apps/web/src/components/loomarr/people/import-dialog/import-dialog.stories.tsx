import type { ImportCandidate } from "@loomarr/api/models/importCandidate";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { ImportDialog } from "./import-dialog";

const mixedCandidates: ImportCandidate[] = [
  { id: "ada", name: "Ada Lovelace", imported: false, disabled: false, isAdmin: true },
  { id: "grace", name: "Grace Hopper", imported: false, disabled: false, isAdmin: false },
  { id: "katherine", name: "Katherine Johnson", imported: false, disabled: true, isAdmin: false },
  { id: "margaret", name: "Margaret Hamilton", imported: true, disabled: false, isAdmin: true },
];

const meta = {
  title: "People/ImportDialog",
  component: ImportDialog,
  args: {
    available: true,
    candidates: mixedCandidates,
    defaultOpen: true,
    onImport: () => {},
    onSync: () => {},
  },
  render: (args) => <ImportDialog {...args} portalContainer={document.getElementById("storybook-root")} />,
} satisfies Meta<typeof ImportDialog>;

type Story = StoryObj<typeof meta>;

const Mixed: Story = {};
const Loading: Story = { args: { candidates: undefined } };
const Empty: Story = { args: { candidates: [] } };
const Disconnected: Story = { args: { available: false, candidates: undefined } };
const AllImported: Story = {
  args: { candidates: mixedCandidates.map((candidate) => ({ ...candidate, imported: true })) },
};
const Busy: Story = { args: { importing: true } };
const ProviderError: Story = {
  args: {
    candidateError: {
      title: "Media server unavailable",
      detail: "Loomarr could not read accounts from Emby. Check the connection and try again.",
    },
  },
};

export default meta;
export { AllImported, Busy, Disconnected, Empty, Loading, Mixed, ProviderError };
