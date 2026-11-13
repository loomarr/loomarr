import { BrandLockup, Disclosure, Surface, Text } from "@loomarr/design-system";
import { type ComponentProps, useState } from "react";

import type { AboutDetail, AboutPageProps } from "./about-page.type";

// ABOUT (Settings → System → About): Loomarr's own lockup and version first, then the data sources
// with the attribution TMDB's terms require (#2853), then the bug-report facts behind one
// disclosure, the way a title's technical details are.

// TMDB_NOTICE is TMDB's attribution sentence, word for word (#2853). ⚠ Do not paraphrase, shorten
// or re-flow it: it is quoted text.
const TMDB_NOTICE = "This product uses the TMDB API but is not endorsed or certified by TMDB.";

const Stack = (props: ComponentProps<typeof Surface>) => (
  <Surface backgroundColor="transparent" borderWidth={0} {...props} />
);

const Row = ({ label, value }: AboutDetail) => (
  <Stack alignItems="flex-start" columnGap="section" flexDirection="row" flexWrap="wrap" rowGap={4}>
    <Stack width={160}>
      <Text textRole="compact" tone="muted">
        {label}
      </Text>
    </Stack>
    <Stack flexBasis={240} flexGrow={1} flexShrink={1} minWidth={0}>
      <Text textRole="data">{value}</Text>
    </Stack>
  </Stack>
);

const AboutPage = ({ version, details, tmdbLogo, demo }: AboutPageProps) => {
  const [open, setOpen] = useState(false);
  return (
    <Stack gap="section" maxWidth={720} width="100%">
      <Surface
        alignItems="center"
        aria-label="Loomarr"
        borderRadius="card"
        gap="control"
        paddingHorizontal="section"
        paddingVertical={40}
        role="region"
      >
        <BrandLockup orientation="stacked" showTagline size="large" />
        <Text textRole="data" tone="secondary">
          {`Version ${version}`}
        </Text>
      </Surface>

      <Stack aria-label="Data sources" gap="control" role="region">
        <Text textRole="section">Data sources</Text>
        <Surface borderRadius="card" gap="control" padding="section">
          {tmdbLogo ? <Stack alignItems="flex-start">{tmdbLogo}</Stack> : null}
          <Text textRole="body">Film and TV details, posters and artwork come from TMDB.</Text>
          <Text textRole="caption" tone="secondary">
            {TMDB_NOTICE}
          </Text>
        </Surface>
        {demo ? (
          // The demo pack's attribution (#3409): each title's Info names its maker and licence,
          // and the source pages are listed once, in the docs, rather than as URLs in every Info.
          <Surface borderRadius="card" gap="control" padding="section">
            <Text textRole="body">
              The demo's films, cartoons and ads are openly licensed. Each one's Info names who made it and
              its licence.
            </Text>
            <Text textRole="caption" tone="secondary">
              Every source is listed on the Demo content page of Loomarr's documentation.
            </Text>
          </Surface>
        ) : null}
      </Stack>

      <Disclosure
        description="Worth including when you report a problem."
        expanded={open}
        label="Technical details"
        onExpandedChange={setOpen}
      >
        <Stack gap="control">
          {details.map((d) => (
            <Row key={d.label} {...d} />
          ))}
        </Stack>
      </Disclosure>
    </Stack>
  );
};

export { AboutPage, TMDB_NOTICE };
