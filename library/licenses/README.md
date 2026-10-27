# Third-party icon notices

## Papirus: Synology Drive

- File: `library/synology/synology-drive.svg`
- Authors: Papirus Development Team and contributors.
- Source: https://github.com/PapirusDevelopmentTeam/papirus-icon-theme/blob/master/Papirus/64x64/apps/synology-drive.svg
- License: GPL-3.0. Full text: [Papirus-GPL-3.0.txt](Papirus-GPL-3.0.txt).
- Modification: set the canvas to 256 × 256 with viewBox 0 0 64 64, normalize XML serialization; preserve vector artwork.

## flag-icons: Hong Kong icon

- File: `library/hk/hk.svg`
- Authors: flag-icons contributors; copyright notice retained in the full license.
- Source: https://github.com/lipis/flag-icons/blob/main/flags/1x1/hk.svg
- License: MIT. Full text: [flag-icons-MIT.txt](flag-icons-MIT.txt).
- Modification: compose the flower paths over a circular red field on a rounded white 256 × 256 canvas.

## SVG Repo: tenki.jp

- File: `library/tenki/tenki.svg`
- Source: https://www.svgrepo.com/svg/519670/tenki-jp
- Source page labels the geometry CC0-1.0: https://creativecommons.org/publicdomain/zero/1.0/
- Modification: restore the blue cube, red sun and white cloud, normalize to a square canvas and add vector corner clipping. Brand trademark rights remain with their respective owner.

## User-requested additions

- Most additions: user-supplied HD-Icons URLs under https://icon.4738.org/icons/HD-Icons/svg/ . Individual source URLs are retained in each icon metadata entry.
- Kimi clean K artwork: official branding assets at https://moonshotai.github.io/Branding-Guide/ . The icon uses the official vector mark with a black rounded background instead of the supplied oversized textured version.
- KFC bucket artwork: SVG file supplied directly by the user. The supplied remote raster wrapper was not imported.
- Processing: safe SVG validation, vector corner clipping where applicable, and SVG optimization. Luckin Coffee's declared CSS background was restored as a vector rectangle for renderer compatibility.
- Trademark and artwork rights remain with their respective owners; these entries do not assert an additional license grant.

## Dashboard Icons

- Source: https://github.com/homarr-labs/dashboard-icons
- License: Apache-2.0. Full text: [dashboard-icons-Apache-2.0.txt](dashboard-icons-Apache-2.0.txt).
- Individual SVG source URLs and original filenames are retained in each icon metadata entry. Editor metadata and non-rendering CSS comments removed from seven SVGs; vector artwork preserved; trademark rights remain with the respective owners.

## Debian Open Use Logo

- File: `library/debian/debian.svg`
- Source: https://www.debian.org/logos/openlogo-nd.svg ; https://www.debian.org/logos/
- Author: Raul Silva. Copyright (c) 1999 Software in the Public Interest, Inc.
- License selected from official dual licensing: CC-BY-SA-3.0, https://creativecommons.org/licenses/by-sa/3.0/ . Trademark rights remain with their owner.
- Modification: removed Adobe editor metadata/attributes; vector artwork and original red color preserved.

## Dashboard Icons 1.0.3 completeness pass

- Upstream commit: `83f32e08257b66fbd971ac8d7645f5b38d52e85d` (2026-10-27 snapshot).
- Added 3212 SVGs; preserved existing versions and defaults. Removed editor-only metadata, CSS comments, unused web fonts, embedded color-profile descriptors and non-rendering scripts where needed. Vector geometry and RGB colors retained.
- 99 upstream files containing raster data, invalid XML or unsupported content excluded; full per-file provenance and exclusion report: `docs/dashboard-icons-import.json`.
- Apache-2.0 repository license remains in `dashboard-icons-Apache-2.0.txt`; artwork and trademark rights remain with their respective owners.

## User-supplied operating-system SVGs (2026-10-27)

- Added 2 Red Hat, 2 Linux and 7 Debian SVG variants supplied directly by the user. Original download filenames and SHA256 hashes are retained in each icon metadata; mapping: `docs/user-os-assets-20261027.json`.
- No separate permission statement accompanied these files; no additional copyright or trademark license grant is asserted. Existing per-icon and upstream notices remain in place.
- Modifications: remove an external DOCTYPE declaration from `2333386-code-linux-os_85584.svg`; remove non-rendering Inkscape/Sodipodi/RDF metadata from `distributorlogodebian_103834.svg`. SVG geometry, transforms, gradients and colors remain unchanged. Nine other supplied SVGs retain their exact original bytes.

## User-requested WinRAR SVG (2026-10-27)

- Source: https://github.com/Razinal/winrar/blob/master/assets/winrar-icon.svg . Original SVG bytes preserved; SHA256 and source retained in metadata.
- Repository code license is MIT; copied as `razinal-winrar-repository-MIT.txt`. No separate artwork license was identified; no additional trademark or artwork license grant is asserted.

## Curated library exclusion (2026-10-27)

- At the owner's request, all PVY and PVY Affiliate artwork was permanently removed (237 SVGs). The upstream synchronizer excludes this brand family, including its 240 upstream file variants; no excluded artwork is redistributed in the materials ZIP.
