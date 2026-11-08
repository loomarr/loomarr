import { BrandLockup, Disclosure, Surface, Text } from "@loomarr/design-system";
import { type ComponentProps, useState } from "react";

import type { AboutDetail, AboutPageProps } from "./about-page.type";

// ABOUT (Settings → System → About): Loomarr's own lockup and version first, then the credits TMDB's
// licence requires, then the bug-report facts behind one disclosure, the way a title's technical
// details are.

// TMDB_NOTICE is fixed wording from TMDB's terms. ⚠ Do not paraphrase, shorten or re-flow it: it is
// quoted text, and "not endorsed, certified, or otherwise approved" is the operative phrase.
const TMDB_NOTICE =
  "This product uses TMDB and the TMDB APIs but is not endorsed, certified, or otherwise approved by TMDB.";

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
    <Stack flexBasis={240} flexGrow={1} minWidth={0}>
      <Text textRole="data">{value}</Text>
    </Stack>
  </Stack>
);

const AboutPage = ({ version, details, tmdbLogo }: AboutPageProps) => {
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

      <Stack aria-label="Credits" gap="control" role="region">
        <Text textRole="section">Credits</Text>
        <Surface borderRadius="card" gap="control" padding="section">
          {tmdbLogo ? <Stack alignItems="flex-start">{tmdbLogo}</Stack> : null}
          <Text textRole="body">Film and TV details, posters and artwork come from TMDB.</Text>
          <Text textRole="caption" tone="secondary">
            {TMDB_NOTICE}
          </Text>
        </Surface>
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
