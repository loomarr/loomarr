import { createFileRoute } from "@tanstack/react-router";
import { useEffect, useRef } from "react";
import { SettingsPage } from "@/settings/settings-page";
import { useSettingsEntries } from "@/settings/use-settings-entries";

const publicURLKey = "access.public_url";

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

const GeneralSettings = () => {
  const entries = useSettingsEntries();
  const publicURL = entries.find((entry) => entry.key === publicURLKey);

  return (
    <SettingsPage
      title="General"
      description="Application-wide identity and addresses used across Loomarr."
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
      ]}
    >
      {({ liveValue, setEdit }) => (
        <PublicURLDefault
          editable={publicURL?.provenance !== "env"}
          loaded={publicURL !== undefined}
          persistedValue={publicURL?.value ?? ""}
          liveValue={() => liveValue(publicURLKey)}
          setEdit={setEdit}
        />
      )}
    </SettingsPage>
  );
};

const Route = createFileRoute("/_authed/settings/general")({
  component: GeneralSettings,
});

export { Route };
