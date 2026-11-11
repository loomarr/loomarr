# Demo content

**For:** anyone running or reviewing the public demo site.
**You'll get:** what the open content pack plays, where each item comes from, and its licence.

The public demo site plays real films, cartoons and vintage commercials instead of the invented
test-card library. Every item is openly licensed: Creative Commons Attribution films from the Blender
Foundation, and public-domain cartoons and screen ads from the Internet Archive. Nothing here is
non-commercial, no-derivatives or share-alike, and everything is suitable for all ages.

The pack is `internal/demolibrary/openpack.json`. It pins each file by URL, SHA-256 and byte size;
no media is committed. On its first start the demo library downloads the files, checks each one
against its hash and refuses to serve anything that doesn't match. The posters and backdrops are
frames cut from the videos themselves. Each title's overview ends with its credit and its licence by
name, which is how the app attributes it. The overview carries no URL, because Info shows it as plain
text; on the demo site, Settings → About says the sources are listed on this page.

The downloads come from a mirror first: the manifest's `mirror` is a `demo-pack-v<N>` release of
this repository, which holds every file under its SHA-256. It is about five times faster than the
Internet Archive, and a deploy doesn't depend on the Archive being up. A file the mirror can't serve
comes from its own URL instead, and the hash check is the same either way. After changing the
pack's files, publish a new mirror and point `mirror` at it:

```bash
scripts/mirror-demo-pack.sh demo-pack-v2
```

Turn it on with `DEMO_SITE_CONTENT=open` (see [Environments](environments.md)). The public site uses
it by default. Tests, screenshots and the docs keep using the invented library, so nothing in them
names a real title.

## Programmes

Five films and one series of six episodes, about 1.2 GB.

| Title | Year | Runtime | Licence | Source |
| --- | --- | --- | --- | --- |
| Big Buck Bunny | 2008 | 10:34 | [CC BY 3.0](https://creativecommons.org/licenses/by/3.0/) | [Blender Foundation](https://peach.blender.org/about/); file from the [Internet Archive](https://archive.org/details/BigBuckBunnyFULLHD60FPS) |
| Wing It! | 2023 | 3:57 | [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/) | [Blender Studio](https://studio.blender.org/projects/wing-it/pages/licensing/); file from the [Internet Archive](https://archive.org/details/wing_it) |
| Caminandes: Llamigos | 2016 | 2:30 | [CC BY 3.0](https://creativecommons.org/licenses/by/3.0/) | [Blender Foundation](https://video.blender.org/videos/watch/23f3ef79-15dc-44c5-aa45-cf92e78a4509); file from the [Internet Archive](https://archive.org/details/CaminandesLlamigos) |
| Popeye the Sailor Meets Sindbad the Sailor | 1936 | 15:43 | [Public Domain Mark](https://creativecommons.org/publicdomain/mark/1.0/) | Fleischer Studios, via the [Internet Archive](https://archive.org/details/popeye-the-sailor-meets-sindbad-the-sailor-1936-1080p) |
| Design for Dreaming | 1956 | 9:16 | [Public domain](https://creativecommons.org/licenses/publicdomain/) | MPO Productions for General Motors, Prelinger Archives, via the [Internet Archive](https://archive.org/details/Designfo1956) |
| Superman: The Mad Scientist | 1941 | 10:20 | [Public Domain Mark](https://creativecommons.org/publicdomain/mark/1.0/) | Fleischer Studios, via the [Internet Archive](https://archive.org/details/superman-the-complete-collection-fleischerfamous-studios) |
| Superman: The Mechanical Monsters | 1941 | 10:11 | [Public Domain Mark](https://creativecommons.org/publicdomain/mark/1.0/) | The same collection |
| Superman: Billion Dollar Limited | 1942 | 8:32 | [Public Domain Mark](https://creativecommons.org/publicdomain/mark/1.0/) | The same collection |
| Superman: The Arctic Giant | 1942 | 8:33 | [Public Domain Mark](https://creativecommons.org/publicdomain/mark/1.0/) | The same collection |
| Superman: The Bulleteers | 1942 | 8:00 | [Public Domain Mark](https://creativecommons.org/publicdomain/mark/1.0/) | The same collection |
| Superman: The Magnetic Telescope | 1942 | 7:33 | [Public Domain Mark](https://creativecommons.org/publicdomain/mark/1.0/) | The same collection |

Credits, as the overviews give them:

- Big Buck Bunny (2008) © Blender Foundation | peach.blender.org.
- Wing It! (2023) (CC) Blender Foundation | studio.blender.org.
- Caminandes: Llamigos (2016) © Blender Foundation | studio.blender.org.
- Popeye the Sailor Meets Sindbad the Sailor (1936) and Superman (1941–1942), Fleischer Studios,
  released by Paramount Pictures.
- Design for Dreaming (1956), MPO Productions for General Motors.

## Filler

Twelve screen ads and theatre trailers from the Prelinger Archives, all
[public domain](https://creativecommons.org/licenses/publicdomain/) and served through the Internet
Archive, about 67 MB. Their ids follow the filler naming convention, a kind and a year, so the
filler pipeline files them as commercials or trailers of their era without a model.

| Clip | Year | Length | Source |
| --- | --- | --- | --- |
| Previews of Coming Attractions | ca. 1940 | 0:20 | [Previews1940](https://archive.org/details/Previews1940) |
| Time Savers for House Makers | ca. 1936 | 1:00 | [TimeSave1936](https://archive.org/details/TimeSave1936) |
| Cheerios/V-8 | 1960 | 1:01 | [Cheerios1960](https://archive.org/details/Cheerios1960) |
| Extra (Esso screen ad) | ca. 1938 | 1:01 | [ExtraEss1938](https://archive.org/details/ExtraEss1938) |
| A Film Mystery (Dreft screen ad) | ca. 1937 | 1:00 | [FilmMyst1937](https://archive.org/details/FilmMyst1937) |
| Nonesuch Coffee ad | ca. 1934 | 1:01 | [Nonesuch1934](https://archive.org/details/Nonesuch1934) |
| Party Magic (Whiz bar screen ad) | ca. 1938 | 1:01 | [PartyMag1938](https://archive.org/details/PartyMag1938) |
| Independence Day, 1940 promotion | 1940 | 0:41 | [MovieTra1940](https://archive.org/details/MovieTra1940) |
| Mother's Day, 1934 promotion | 1934 | 0:49 | [MovieTra1934](https://archive.org/details/MovieTra1934) |
| She Caught on Quick (Singer screen ad) | 1940 | 1:20 | [SheCaugh1940](https://archive.org/details/SheCaugh1940) |
| The Trip (Chevrolet screen ad) | 1939 | 0:50 | [TripThe1939](https://archive.org/details/TripThe1939) |
| Oldsmobile Presents Motoring's Magic Carpet | 1941 | 0:50 | [Oldsmobi1941](https://archive.org/details/Oldsmobi1941) |

## Channels

| Channel | Plays |
| --- | --- |
| 101 Saturday Morning Cartoons | Superman, Popeye and the three Blender films, shuffled |
| 102 Open Movie Night | The three Blender films, in order |
| 103 Retro Rewind | Superman, Popeye and Design for Dreaming, shuffled |

## Adding an item

Every item needs a licence the pack accepts (CC BY, the Public Domain Mark, or the Prelinger public
domain dedication), the page you verified that licence on, a credit, and the all-ages flag. Record
the file's SHA-256, byte size and probed shape from the file you downloaded, not from the host's
metadata. The pack's tests reject anything missing, and the whole pack stays under 4 GB. Add the
item to this page in the same change.

Left out on purpose: Tears of Steel and Sintel (violence), Cosmos Laundromat (themes), Agent 327 and
Coffee Run (no-derivatives licences), Caminandes 1 and 2 (no CC BY licence stated by the Blender
Foundation), and the wartime Superman and Popeye shorts that carry racist caricatures.
