# Third-party components

The project-authored Go source is MIT licensed. Third-party material retains its original license.

| Component | Use | License / source |
| --- | --- | --- |
| PawnIO.Modules 0.2.11 AMDFamily17.bin | Unmodified signed module embedded for CPU temperature reads | LGPL-2.1-or-later; license, full matching module source archive and provenance in `third_party/pawnio/` |
| PawnIO 2.2.0 driver | Separately installed Windows dependency, not distributed by the app | Upstream driver license and official signed distribution at https://pawnio.eu/ |
| Go standard library/runtime | Compiler/runtime and standard packages | Go BSD-style license at https://go.dev/LICENSE |
| AMD ADL | Calls the user's installed Windows graphics driver | No AMD DLL or SDK library is bundled; API references in source and docs |

The original JONSBO application, decompiled vendor source, captured USB traffic, downloaded compilers and driver installers are local research/development artifacts, excluded from version control and release bundles. No HWiNFO/JONSBO DLL is linked or loaded by this app. This project is independent and is not endorsed by JONSBO, AMD, OpenAI, or Anthropic.

The module's matching source and license must accompany distributed app binaries. Do not relabel third-party files as MIT. See `third_party/pawnio/README.md` for module hashes and replacement/rebuild instructions.
`go-winres` v0.3.3 is a pinned build-only resource generator, not linked into the app. Its generated icon and version resources are built from the maintainer-provided signature artwork under `assets/`.
