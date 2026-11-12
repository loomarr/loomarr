import * as authApi from "@loomarr/api/endpoints/auth";
import * as systemApi from "@loomarr/api/endpoints/system";
import { SettingKey } from "@loomarr/api/models/settingKey";
import { Disclosure, Stack, Text } from "@loomarr/design-system";
import { createFileRoute } from "@tanstack/react-router";
import { useEffect, useRef } from "react";
import { viewerIsAdmin } from "@/auth/me-query";
import { InstallationLocation } from "@/components/loomarr/settings/installation-location";
import { EncryptionSettings } from "@/settings/encryption-settings";
import { PairedDevices } from "@/settings/paired-devices";
import { SecretsSettings } from "@/settings/secrets-settings";
import { SettingsPage } from "@/settings/settings-page";
import { useSettingsEntries } from "@/settings/use-settings-entries";

const publicURLKey = SettingKey.AccessPublicURL;

const SsoNote = () => (
  <section className="border-border rounded-lg border p-4">
    <Text as="h2" textRole="cardLabel">
      How sign-in works with a provider
    </Text>
    <Text as="p" className="mt-1" textRole="compact" tone="secondary">
      Your provider proves who someone is. The People list decides whether they can get in — signing in with
      your provider does not create an account here, and it does not set anyone's role. Add people under
      People first, then they can sign in either way.
    </Text>
  </section>
);

interface PublicURLDefaultProps {
  editable: boolean;
  loaded: boolean;
  persistedValue: string;
  liveValue: () => string;
  setEdit: (key: SettingKey, value: string) => void;
}

const PublicURLDefault = ({
  editable,
  loaded,
  persistedValue,
  liveValue,
  setEdit,
}: PublicURLDefaultProps) => {
  const considered = useRef(false);

  useEffect(() => {
    if (!loaded || considered.current) return;
    considered.current = true;
    if (!editable) return;
    if (persistedValue.trim() !== "" || liveValue().trim() !== "") return;
    if (window.location.protocol !== "http:" && window.location.protocol !== "https:") return;
    setEdit(publicURLKey, window.location.origin);
  }, [editable, liveValue, loaded, persistedValue, setEdit]);

  return null;
};

const AccessSettings = () => {
  const entries = useSettingsEntries();
  const publicURL = entries.find((entry) => entry.key === publicURLKey);

  return (
    <SettingsPage
      title="Access and devices"
      description="Manage how people and devices reach Loomarr, sign in, and set your household defaults."
      entries={entries}
      blocks={[
        {
          group: "general",
          title: "Share invitation and recovery links",
          description:
            "Defaults to the browser address you're using now. Change it if recipients reach Loomarr at a different address. Loomarr uses the saved value for copied links, QR codes, and account email.",
          keys: [publicURLKey],
          surface: "card",
        },
        {
          group: "users_security",
          title: "Sign-in sessions",
          description:
            "Choose how long sign-ins last. Loomarr's automatic secure-cookie policy is the recommended default.",
        },
        {
          group: "sso",
          title: "Single sign-on",
          description: "Optional. Let existing Loomarr people prove their identity with your provider.",
        },
      ]}
      footer={
        <>
          <SsoNote />
          <PairedDevices />
          <Disclosure
            description="Generated credentials and database encryption for unusual administration work."
            label="Advanced access controls"
          >
            <Stack gap="4">
              <SecretsSettings />
              <EncryptionSettings />
            </Stack>
          </Disclosure>
        </>
      }
    >
      {({ liveValue, setEdit }) => (
        <>
          <InstallationLocation
            entries={entries}
            values={{
              [SettingKey.FillerHomeCountry]: liveValue(SettingKey.FillerHomeCountry),
              [SettingKey.FillerHomePlace]: liveValue(SettingKey.FillerHomePlace),
              [SettingKey.FillerLocalRadiusKm]: liveValue(SettingKey.FillerLocalRadiusKm),
            }}
            onChange={setEdit}
            card
          />
          <PublicURLDefault
            editable={publicURL?.provenance !== "env"}
            loaded={publicURL !== undefined}
            persistedValue={publicURL?.value ?? ""}
            liveValue={() => liveValue(publicURLKey)}
            setEdit={setEdit}
          />
        </>
      )}
    </SettingsPage>
  );
};

const Route = createFileRoute("/_authed/settings/access")({
  component: AccessSettings,
  // The paired devices and the encryption status each part of the page reads for itself and
  // answers for when it fails, so they are warmed (a prefetch never rejects). The settings list is
  // the layout's. Admin-only, like the page.
  loader: async ({ context: { queryClient } }) => {
    if (!(await viewerIsAdmin(queryClient))) return;
    await Promise.all([
      queryClient.prefetchQuery(authApi.getDeviceListQueryOptions()),
      queryClient.prefetchQuery(systemApi.getSystemEncryptionStatusQueryOptions({ query: { retry: false } })),
    ]);
  },
});

export { Route };
