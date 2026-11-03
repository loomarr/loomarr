import * as fillerApi from "@loomarr/api/endpoints/filler";
import type { FillerProviderDescriptionDTO } from "@loomarr/api/models/fillerProviderDescriptionDTO";
import { unwrap } from "@loomarr/api/unwrap";

// The id the provider routes (`/v1/filler/providers/{kind}`) accept, as the generated client types it.
type RemoteProviderKind = Parameters<typeof fillerApi.setFillerProviderEnabled>[0];

// A provider whose sources are fetched, suggested and verified by address: the ones the provider
// routes act on.
type RemoteProvider = FillerProviderDescriptionDTO & { id: RemoteProviderKind };

// The registered source providers, from the backend registry: what each is called and what it can
// do. The web never names a provider; it asks this.
//
// ⚠ Absent until the read answers, so a caller gating UI on a capability shows nothing for the
// first beat rather than guessing.
const useFillerProviders = () => {
  const query = fillerApi.useListFillerProviders();
  const providers = unwrap(query.data, (body) => body.providers) ?? [];

  const find = (id: string) => providers.find((provider) => provider.id === id);

  return {
    find,
    /**
     * The provider when it can suggest sources and verify an address, which is what the provider
     * routes serve. The narrowing is the one place the registry's string id meets the route's
     * generated path type.
     */
    discoverable: (id: string) => {
      const provider = find(id);
      return provider?.discoverable ? (provider as RemoteProvider) : undefined;
    },
  };
};

export type { RemoteProvider, RemoteProviderKind };
export { useFillerProviders };
