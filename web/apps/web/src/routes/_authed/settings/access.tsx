import { createFileRoute } from "@tanstack/react-router";
import { useEffect, useRef } from "react";
import { CollapsibleSection } from "@/components/loomarr/feedback/collapsible-section";
import { InstallationLocation } from "@/components/loomarr/settings/installation-location";
import { EncryptionSettings } from "@/settings/encryption-settings";
import { PairedDevices } from "@/settings/paired-devices";
import { SecretsSettings } from "@/settings/secrets-settings";
import { SettingsPage } from "@/settings/settings-page";
import { useSettingsEntries } from "@/settings/use-settings-entries";

const publicURLKey = "access.public_url";

const SsoNote = () => (
  <section className="rounded-lg border border-border p-4">
    <h2 className="font-medium text-sm">How sign-in works with a provider</h2>
    <p className="mt-1 text-muted-foreground text-sm leading-relaxed">
      Your provider proves who someone is. The People list decides whether they can get in — signing in with
      your provider does not create an account here, and it does not set anyone's role. Add people under
      People first, then they can sign in either way.
    </p>
  </section>
);

interface PublicURLDefaultProps {
  editable: boolean;
  loaded: boolean;
  persistedValue: string;
  liveValue: () => string;
  setEdit: (key: string, value: string) => void;
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
          <CollapsibleSection
            title="Advanced access controls"
            description="Generated credentials and database encryption for unusual administration work."
          >
            <div className="flex flex-col gap-4">
              <SecretsSettings />
              <EncryptionSettings />
            </div>
          </CollapsibleSection>
        </>
      }
    >
      {({ liveValue, setEdit }) => (
        <>
          <InstallationLocation
            entries={entries}
            values={{
              "filler.home_country": liveValue("filler.home_country"),
              "filler.home_market": liveValue("filler.home_market"),
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
});

export { Route };
