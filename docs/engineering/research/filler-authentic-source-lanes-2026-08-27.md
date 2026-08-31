# Authentic filler source lanes for schema v5

**Decision date:** 2026-08-27

**Status:** acquisition decision for the 1,426-case corpus, including the 446 eligible-positive holdout

This report evaluates acquisition lanes for authentic commercials, promos, bumpers, station IDs,
trailers, and PSAs. It uses owner-published licences, official agency policies, and first-party API
documentation. No owner was contacted, no media was downloaded, and no purchase was made. This is an
engineering rights policy, not legal advice.

## Decision

The corpus cannot be completed from a public repository or stock library. Loomarr must make a
rights-complete direct creator/broadcaster contribution programme the backbone and treat public-domain
or openly licensed collections as item-reviewed supplements.

The direct agreement must grant the complete evaluation use, not merely permission to view or air a
clip. It must cover commercial copying, storage, hashing, transcoding, bounded frame/audio/text
extraction, automated analysis, and a transferable/sublicensable processing grant for named
model-evaluation processors, as well as redistribution of the exact master and permitted derivatives
and publication of provenance and results. The signer
must identify the creator, campaign, source master, derivative family, and every third-party element
or warrant authority over them. A downloadable file and a licence label are not substitutes for that
authority.

This is not optional quota padding. The locked corpus requires 300 development cases plus 1,126
independent holdout cases: 446 eligible positives, 446 deterministic rejects, 147 semantic rejects,
and 87 ambiguous cases. All must be authentic, rights-complete media. Deliberately corrupting or
relabeling one acquired master does not create another independent holdout family.

The eligible-positive holdout has these exact role quotas:

| Role | Cases | Maximum from one creator | Minimum creators |
| --- | ---: | ---: | ---: |
| Commercial | 82 | 8 | 11 |
| Promo | 82 | 8 | 11 |
| Bumper | 59 | 5 | 12 |
| Station ID | 59 | 5 | 12 |
| Trailer | 82 | 8 | 11 |
| PSA | 82 | 8 | 11 |

The integer maxima follow directly from the 10% per-role creator cap. The 25% source cap allows at
most 111 of 446 eligible cases under one `source` value, so at least five independent source
authorities are necessary. Each campaign, source master, source family, and similarity cluster may
contribute only one holdout case. A 60-, 30-, and 15-second cut of one campaign is therefore one
independent opportunity, not three.

For direct contributions, `source` must identify the contracting owner or first-party origin, not a
shared upload transport called `direct`. Otherwise a successful direct programme would fail the
source-concentration gate despite having independent owners.

## Required rights bundle

The acquisition reviewer must answer every column below for the exact representation. An asset is
held when any answer is unknown.

| Right or fact | Required evidence |
| --- | --- |
| Acquisition | Owner-controlled direct download or an owner-delivered master, bound to a checksum |
| Model-provider upload | Express, transferable/sublicensable permission for automated evaluation and third-party processing; run only through zero-data-retention, no-training endpoints |
| Modification | Permission for technical transcoding and bounded frame, audio, transcript, thumbnail, and evidence-packet extraction |
| Redistribution | Permission to retain and redistribute the master and allowed derivatives with the corpus; a private-viewing or broadcast-only grant fails |
| Commercial reuse | Express commercial permission or a rights basis that permits it; non-commercial and donated-media-only grants fail |
| Embedded rights | Authority over music, performers, stock footage, artwork, marks, privacy/publicity, and locations, or an explicit item-level exclusion and hold |
| Identity | Stable creator, campaign, master, source-family, and owner-authority identifiers |
| Duration | Worldwide and perpetual or irrevocable for the corpus lifetime; expiry-based placements fail |

OpenRouter sends prompts through multiple processing points and provider policies differ. More
importantly, its consumer terms require the user to grant OpenRouter a transferable, sublicensable,
worldwide licence to host, reproduce, transmit, distribute, and format the input, and require the user
to warrant authority for that grant ([OpenRouter terms, section 6](https://openrouter.ai/terms)). Its
official documentation supports per-request zero-data-retention routing and says ZDR providers neither
retain nor train on request data ([ZDR documentation](https://openrouter.ai/docs/guides/features/zdr),
[data collection](https://openrouter.ai/docs/guides/privacy/data-collection)). ZDR reduces disclosure
risk; it does not narrow the licence promised in the terms or create copyright permission. Every
source agreement must authorize that provider grant. A separately negotiated enterprise term may
change the analysis, but it must be reviewed and frozen before use.

CC0 is the strongest generally published Creative Commons path for provider use: its waiver and
fallback licence cover any purpose, including commercial, advertising, and promotional uses, subject
to rights the affirmer does not own
([CC0 legal code](https://creativecommons.org/publicdomain/zero/1.0/legalcode.en)). CC BY 4.0 permits
commercial sharing and adaptation with attribution and change notices
([CC BY 4.0](https://creativecommons.org/licenses/by/4.0/)), but its legal code makes that licence
non-sublicensable ([CC BY legal code](https://creativecommons.org/licenses/by/4.0/legalcode.en)). That
conflicts with the grant OpenRouter requires the Loomarr user to make. Creative Commons also cautions
that AI input preparation normally involves copying and that licence conditions depend on the actual
use and jurisdiction
([CC guidance for AI training](https://creativecommons.org/using-cc-licensed-works-for-ai-training-2/)).
The affirmer must actually own the rights and rights review must still confirm that the OpenRouter
representation can be made. Accordingly, CC BY or BY-SA alone may support local corpus work but fails `modelProviderUse` for this
OpenRouter route. It needs a separate owner-signed provider-processing grant. Reject NC and ND
licences entirely. No CC tool clears trademarks, publicity, privacy, or embedded third-party works.

## Ranked acquisition matrix

`Yes` means the lane can supply the right only after the stated item-level check. `No` means its
published terms conflict with the corpus contract. Counts are acquisition ceilings or planning
targets, not approved inventory.

| Rank | Lane | Useful roles and plausible holdout contribution | Direct master | Provider upload | Modify | Redistribute | Commercial | Creator/campaign identity | Decision |
| ---: | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 1 | Executed creator/broadcaster contribution agreement | Eligible roles plus authentic reject and ambiguous controls; recruit against all 1,426 cases, with the direct lane as the backstop for every public-source shortfall | Yes | Yes, by contract | Yes, by contract | Yes, by contract | Yes, by contract | Required in schedule | **Backbone** |
| 2 | Exact first-party works from CDC, SAMHSA, NASA, or another producing federal agency | PSAs first; some promos and trailers; no credible path to 59 bumpers or 59 IDs | Usually | No on US public-domain status alone | Agency-specific | Conditional | Conditional | Agency/item IDs usually available; campaign often manual | **Supplement only after worldwide provider grant** |
| 3 | Exact Prelinger item with a qualifying item-page CC/public-domain basis | Historical commercials, PSAs, and a small promo pool; maximum 111 across the source, but approved yield is unproven | Yes | Only CC0 or an express provider grant | Licence-specific | Licence-specific | Licence-specific | Creator/campaign often requires manual research | **Supplement only** |
| 4 | LOC National Screening Room exact item | Historical ads, PSAs, trailers, and controls; only pre-1931 or item-cleared works are straightforward | Yes | No on US expiry alone | Conditional | Conditional | Conditional | Catalog identity good; campaign/creator completeness varies | **Local supplement; hold from provider** |
| 5 | Verified creator-owned CC0/BY/BY-SA master on the creator's own site or download-enabled Vimeo | Modern commercials, promos, and trailers; potentially bumpers/IDs from official broadcaster accounts | Conditional | CC0 yes; BY/BY-SA needs separate grant | Licence-specific | Licence-specific | Licence-specific | Creator exposed; campaign/master must be recorded manually | **Conditional; signed grant preferred** |
| 6 | NARA, Wikimedia Commons, Europeana, Open Images, or another aggregator/archive | Discovery across roles; at most exact, owner-verified items count | Often | Conditional | Licence-specific | Licence-specific | Licence-specific | Frequently incomplete or uploader-supplied | **Discovery, not authority** |
| 7 | Blender Open Movies | A few trailers/promos and development fixtures | Yes | Conditional | Licence-specific | Licence-specific | Licence-specific | One dominant creator and few independent films | **Reject as quota lane** |
| 8 | YouTube, studio press/streaming pages, Ad Council, NHTSA campaign delivery, or restricted stock | Attractive previews across roles | No reliable authority | No | No | No | No or placement-limited | Often visible but not licensable | **Reject** |

## Lane findings

### 1. Direct creator and broadcaster agreements

This is the only lane capable of supplying authentic current bumpers and station IDs while also
satisfying the campaign, creator, and source-family controls. It is also the only predictable way to
obtain modern commercial, entertainment-promo, and trailer masters with whole-work authority.

Use a contribution schedule with one row per exact master and require:

- owner legal name, signer authority, creator, campaign, source family, original filename, duration,
  format, delivery URL, and checksum;
- a worldwide, perpetual, irrevocable, royalty-free, non-exclusive commercial grant to reproduce,
  modify technically, analyze, and make the transferable/sublicensable provider-processing grant
  required by the evaluation service, as well as redistribute and publish evidence;
- a warranty or explicit schedule covering music, performers, stock, artwork, marks, and
  privacy/publicity releases;
- permission for automated classification and model evaluation, including provider upload, while
  prohibiting provider retention/training operationally through ZDR;
- no placement window, territory-only broadcast condition, confidentiality term, or downstream
  prohibition incompatible with the corpus; and
- a withdrawal rule limited to proven rights defects, with the exact case retired rather than kept
  through a legacy exception.

Recruit to the full 1,426-case corpus until public supplements are actually approved. The minimum
creator counts for eligible positives are floors, not targets. A practical safer target is at least
15 creators per eligible role, with no creator supplying more than five eligible cases in any role
and no contracting source approaching 111 eligible cases. The same programme must recruit distinct,
authentic programme excerpts, compilations, source-policy failures, unusable media, and genuinely
ambiguous boundaries for the reject and review denominators; these are not synthetic filler
imitations. Broadcasters should preferentially contribute independent promo, bumper, and station-ID
campaigns; advertisers, agencies, independent filmmakers, game studios, nonprofits, and public
agencies should cover commercials, trailers, promos, PSAs, and the named control classes.

### 2. Federal first-party works

United States Government works made by officers or employees in their official duties are not
copyright-protected in the United States, but the government may own transferred copyrights and a
page may contain contractor work ([17 USC 101 and 105](https://www.copyright.gov/title17/92chap1.html)).
That domestic public-domain rule does not itself authorize the worldwide, sublicensable licence
OpenRouter requires. Federal hosting is therefore not enough; the item must identify the producing
agency and be checked for contractors, music, stock, people, marks, foreign rights, and an express
provider-processing basis.

- **CDC:** Most CDC-authored material is public domain and reusable with attribution,
  non-endorsement, no substantive change, and a free-source notice; contractor, grantee, licensed,
  state/local, and foreign material are exceptions
  ([CDC agency-material policy](https://www.cdc.gov/other/agencymaterials.html)). First-party
  downloadable disaster PSAs can qualify for local corpus work after exact-media review, but need a
  separate worldwide provider-use basis. CDC's media repository also
  contains partner ads whose agreements restrict for-profit use and changes, so those do not inherit
  CDC's public-domain status ([CDC MCRC FAQ](https://www.cdc.gov/tobacco/php/multimedia-and-tools/mcrc-faq.html)).
  Even a large approved CDC pool can contribute only eight of the 82 PSA cases under one creator.
- **SAMHSA:** Some pages provide broadcast-quality downloads and expressly say their spots are free,
  created for public use, and usable without permission
  ([Know the Risks media](https://www.samhsa.gov/substance-use/learn/risks/multimedia)). Other
  campaigns impose US-broadcast, non-episodic, no-monetary-gain, and no-alteration restrictions and
  must be rejected
  ([988 You Matter terms](https://www.samhsa.gov/resource/988/you-matter-psa-video)). Treat each
  campaign page as a separate rights decision, not as an agency-wide licence.
- **NASA:** NASA media is generally reusable, but identifiable people, third-party material,
  insignia, endorsement, and special AI rules remain. NASA says its insignia may not appear in AI
  training and requires source disclosure without implying permission or attribution of model output
  to NASA ([NASA media and AI guidelines](https://www.nasa.gov/nasa-brand-center/images-and-media/)).
  Only exact videos without prohibited or uncleared elements can supplement local trailers or promos;
  provider use still needs an authority that satisfies the OpenRouter grant. One NASA creator can
  supply no more than eight cases per 82-case role.
- **NARA:** The Catalog API exposes archival descriptions and digital-object metadata, but NARA says
  Special Media holdings can contain copyrighted, donor-restricted, publicity, and other protected
  material ([NARA permissions](https://www.archives.gov/research/motion-pictures/permissions)). Its
  API terms also prohibit caching API content and say it may not be modified while still represented
  as NARA-sourced ([Catalog API terms](https://www.archives.gov/research/catalog/help/api)). Use the
  API for discovery, then bind an exact federal-origin record and media file outside an API-cache
  assumption. Multiple originating agencies may help creator diversity; semantic role yield remains
  unproven.

NHTSA is not an acceptable shortcut. Its official campaign page says video ads may be available only
by request and may not be altered, edited, or changed
([Drive Sober campaign](https://www.trafficsafetymarketing.gov/safety-topics/drunk-driving/drive-sober-or-get-pulled-over)).
That conflicts with evidence extraction and reproducible redistribution.

### 3. Prelinger and LOC historical media

Prelinger says qualifying films may be downloaded, reproduced, redistributed, modified, and sold
according to the exact item-page Creative Commons licence, while its descriptions and shot lists are
separately copyrighted and Internet Archive does not issue a written rights agreement
([Prelinger reuse policy](https://archivesupport.zendesk.com/hc/en-us/articles/360004715031-Prelinger-Archive)).
Internet Archive requires the user to make an independent non-infringement decision
([Archive rights guidance](https://archivesupport.zendesk.com/hc/en-us/articles/360014759692-Rights)).
The lane can contribute only after review of the exact item, licence, original file, publication
facts, embedded works, original creator/campaign identity, and provider-use authority. A US public-
domain mark or ordinary CC BY/BY-SA grant is insufficient for the OpenRouter terms. The prior pilot
did not establish an approved scalable yield, so this remains a supplement rather than a numerical
commitment.

The Library of Congress says it is unaware of restrictions for most National Screening Room items,
but places rights assessment on the user and flags privacy, publicity, licensing, trademark,
donation, US, and foreign-rights uncertainty
([National Screening Room rights](https://www.loc.gov/collections/national-screening-room/about-this-collection/rights-and-access/)).
As of 2026, works published in the United States before 1931 have expired into the US public domain
([Copyright Office](https://www.copyright.gov/what-is-copyright/)). Accept exact pre-1931 or otherwise
item-cleared works for local corpus use; do not infer worldwide provider permission from US expiry or
library possession.

### 4. Verified open creator masters

Vimeo exposes a video's selected CC licence and owner metadata
([video response](https://developer.vimeo.com/api/reference/response/video)), but direct API file
links require an authenticated application, appropriate scopes, and a qualifying paid account
([file-link requirements](https://developer.vimeo.com/api/files/video-links)). Vimeo's terms say
uploaders retain ownership and can enable streaming, embedding, downloads, and transcoding
([Vimeo terms](https://vimeo.com/legal/)); those platform permissions do not prove that an arbitrary
uploader owns music, performances, brands, or the whole work.

Accept an exact CC0 file from a verified creator/rightsholder with downloading enabled, complete
provenance, campaign/master identity, and an embedded-rights review. Treat BY/BY-SA as local-only
until the owner signs the transferable/sublicensable provider-processing grant. Vimeo itself warns
that it cannot guarantee an uploader has all necessary rights
([Vimeo CC reuse guidance](https://help.vimeo.com/hc/en-us/articles/12427604972305-I-want-to-use-a-Creative-Commons-video-I-saw-on-Vimeo-Do-I-need-permission)).
A signed grant converts the asset to rank 1. Search results, a profile name, and an uploader-set CC
field alone are insufficient.

Blender's open productions are authentic films, and Blender publishes open-film history and a finite
film catalogue ([Blender history](https://www.blender.org/about/history/),
[film catalogue](https://studio.blender.org/films/)). They remain one concentrated production family
with too few independent trailer campaigns to meet the role quota, and the prior distinct-candidate
pilot failed. Use clearly licensed exact assets only for development fixtures or a small supplement,
not as an acquisition lane.

### 5. Aggregators and restricted delivery systems

Wikimedia Commons permits commercial reuse under each file's licence but owns almost none of the
media, provides no rights warranty, and tells reusers to verify every file independently
([Commons reuse guidance](https://commons.wikimedia.org/wiki/Commons:Reusing_content_outside_Wikimedia)).
Europeana likewise passes through provider rights statements and says it does not own or guarantee
the objects ([Europeana terms](https://www.europeana.eu/en/rights/terms-of-use)). The Netherlands
Institute for Sound & Vision says only a small part of its collection is freely reusable through Open
Images and that conditions vary, including non-commercial restrictions
([collection access](https://www.beeldengeluid.nl/en/collection)). These are candidate-discovery
systems. Count an item only after reaching the first-party owner and exact qualifying licence.

Ad Council assets are placement inventory, not corpus media. Its terms limit PSA downloads to
promoting the named issue, prohibit commercial gain and other purposes, and otherwise prohibit
reproduction, adaptation, modification, and distribution without written consent
([Ad Council terms](https://www.adcouncil.org/terms-of-use)). Its media guidance adds US-only use,
donated-media placement, no modification, and expiration dates
([media planning terms](https://www.adcouncil.org/find-assets/media-planning)). Reject the lane.

YouTube is also not an acquisition source. Its developer policies prohibit downloading outside the
allowed playback experience, redistributing audiovisual content, and separating or modifying audio
or video components
([YouTube API policy](https://developers.google.com/youtube/terms/developer-policies)). A creator's
YouTube page can be a discovery lead, but the master and rights must arrive through a separate
owner-authorized channel.

Studio press sites, subscription streaming pages, broadcast archives, and commercial stock libraries
are rejected unless an exact asset carries an owner grant satisfying every required-rights row.
Viewing, embedding, editorial press use, paid placement, an API response, or a stock download licence
does not authorize provider evaluation plus corpus redistribution.

## Concrete acquisition plan

1. Freeze a one-page contribution agreement and per-master schedule containing the complete rights
   bundle above, including authority for OpenRouter's transferable/sublicensable input licence.
   Legal/rights review must approve the template before outreach or acquisition.
2. Build a 1,426-master recruitment ledger with a separately planned 300-case development cohort and
   the exact 446/446/147/87 holdout denominators. For eligible positives, recruit at least 15 creators
   per role. Prioritize at least 12 independent broadcasters for promos, bumpers, and station IDs;
   at least 15 advertisers/agencies for
   commercials; at least 15 independent film/game creators for trailers and promos; and at least 15
   nonprofits or qualifying public agencies for PSAs.
3. Request one distinct campaign master per provisional holdout row. Collect extra masters for the
   300-case development set separately; do not split variants or derivatives across cohorts.
4. Keep the direct target at all 1,426 cases until item-level public review produces locked approvals.
   Substitute approved public cases only when role, creator, source, campaign, and source-family caps
   remain green.
5. Run a rights intake before download. Reject missing signer authority, incomplete embedded-rights
   schedules, NC/ND or placement-only terms, expiry, no-modification clauses, private-only files, or
   provider-transfer ambiguity.
6. After approval, acquire the owner master from the authorized URL, hash it immediately, freeze the
   rights evidence and metadata, and assign creator/campaign/source-family identities before corpus
   split assignment.
7. Execute bakeoff traffic only after counsel confirms the exact OpenRouter and selected model terms,
   with ZDR and data-collection denial enabled. Operational privacy is a second gate, never a
   replacement for source permission.

## Exit criteria

Acquisition is complete only when the locked candidate inventory contains at least the role counts
above, no creator exceeds the per-role integer maximum, no source exceeds 111 eligible cases, and no
campaign/master/family/cluster repeats in holdout. Every row must have a checksum-bound rights record
that independently authorizes acquisition, commercial use, technical modification, the exact
transferable/sublicensable provider grant, and redistribution. Anything else remains a discovery lead
or local development fixture and cannot be promoted through a legacy exception.
