# Issues — 2026-09-28-slice-boundary-guard

Source: [prd.md](./prd.md)

| Done | # | Title | Blocked by |
|------|---|-------|------------|
| [x]  | 1 | [Prefactor: route both check runs through one verification step](./issues/01-prefactor-one-verification-step.md) | None |
| [x]  | 2 | [List earlier slice contributions in each slice prompt](./issues/02-list-earlier-contributions-in-prompt.md) | None |
| [x]  | 3 | [Fail and repair an abandoned declaration](./issues/03-fail-and-repair-abandoned-declaration.md) | [#1](./issues/01-prefactor-one-verification-step.md), [#2](./issues/02-list-earlier-contributions-in-prompt.md) |
| [x]  | 4 | [Exempt a declaration a later slice will use](./issues/04-exempt-declaration-used-later.md) | [#3](./issues/03-fail-and-repair-abandoned-declaration.md) |
| [x]  | 5 | [Catch a slice that deletes an earlier slice file](./issues/05-catch-deleted-earlier-slice-file.md) | [#3](./issues/03-fail-and-repair-abandoned-declaration.md) |
| [x]  | 6 | [Publish the runtime release](./issues/06-publish-runtime-release.md) | [#3](./issues/03-fail-and-repair-abandoned-declaration.md), [#4](./issues/04-exempt-declaration-used-later.md), [#5](./issues/05-catch-deleted-earlier-slice-file.md) |
| [ ]  | 7 | [Bump the minions pin in the cli workflows](./issues/07-bump-minions-pin-in-cli.md) | [#6](./issues/06-publish-runtime-release.md) |

> **Row 6 decision**: `v0.0.14` was published at `57e5313` on 2026-09-29. Release: https://github.com/partio-io/minions/releases/tag/v0.0.14
