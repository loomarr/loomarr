import * as fillerApi from "@loomarr/api/endpoints/filler";
import type { FillerProviderDescriptionDTO } from "@loomarr/api/models/fillerProviderDescriptionDTO";
import { unwrap } from "@loomarr/api/unwrap";

// The registered source providers, from the backend registry: what each is called, what it can do
// and the words the source page shows for it. The web never names a provider; it asks this.
//
// ⚠ Empty until the read answers, so a caller gating UI on a capability shows nothing for the first
// beat rather than guessing.
const useFillerProviders = () => {
  const query = fillerApi.useListFillerProviders();
  const providers = unwrap(query.data, (body) => body.providers) ?? [];

  const find = (id: string): FillerProviderDescriptionDTO | undefined =>
    providers.find((provider) => provider.id === id);

  return {
    providers,
    find,
    /** The provider when it can suggest sources and verify an address, which is what a finder needs. */
    discoverable: (id: string) => {
      const provider = find(id);
      return provider?.discoverable ? provider : undefined;
    },
    /** The providers an operator adds a source to by typing its address into a field, as listed. */
    typed: providers.filter((provider) => provider.addable && !provider.fetched),
  };
};

export { useFillerProviders };
