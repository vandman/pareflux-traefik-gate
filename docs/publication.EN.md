<!--
FR: Décrit la préparation automatique des liens publics avant publication du plugin.
EN: Describes automatic public-link preparation before publishing the plugin.
Called by: maintainers; Calls: publish-github.sh and catalog-links.
-->
# Publishing the catalog documentation

[Français](publication.FR.md) · [Back to the guide](../README.md)

The Traefik catalog displays the public release README on its own website.
A relative link such as `LISEZMOI.md` would therefore point at the catalog,
not at the repository. Publication prepares only the README displayed by
Traefik instead of requiring manual replacements. Other documents remain on
GitHub and retain their relative links. Links to other documents leave the catalog.

## Publish a release

Commit the validated source and use an unused stable version tag. The primary
GitLab release tag is `publish/vX.Y.Z`; the public GitHub tag is `vX.Y.Z`.
Existing public tags are immutable. Run the checks before the actual publication:

```bash
PUBLISH_DRY_RUN=1 ./scripts/publish-github.sh vX.Y.Z
./scripts/publish-github.sh vX.Y.Z
```

The publisher works in an isolated export; it never rewrites the source checkout.
Git, Bash and either Go or Docker are required. A configured protected CI deploy
key can publish through the release pipeline; otherwise the maintainer uses the
workstation after the release checks succeed.

## Link preparation

`scripts/catalog-links` is a standard-library Go utility invoked automatically
by the publisher, before its existing compilation and license checks:

- Within the README only, documentation files link to GitHub `blob/<release>/...` pages. Browsing links use `www.github.com`, which redirects to canonical GitHub pages and avoids the catalog renderer rewriting `github.com` links to raw downloads.
- Directories link to GitHub `tree/<release>/...` pages.
- Raw repository roots used as navigation links are converted to repository pages; raw image targets must name a file.
- Images use complete `raw.githubusercontent.com/<owner>/<repository>/<release>/<file>` URLs.
- Relative paths, language links, references, HTML links and contents links are resolved from their source document.
- Repository links pointing at `main` or `master` are pinned to the release.
- Missing local targets, missing Markdown headings, paths outside the repository and incomplete raw URLs stop publication.
- Code examples, inline code and links to sections within the README are preserved.
- No other documentation file is rewritten.
- External websites and the GitLab development-repository link remain unchanged; their availability is not a local-file check.

A raw repository root is not a usable download URL. Use the GitHub repository
page to browse files, or specify both the version and the complete file path
for raw content. The publisher verifies all local README targets before writing the
converted README, then runs a second pass in check-only mode.

The catalog indexes GitHub asynchronously. Publishing a tag does not immediately
refresh the catalog page; verify the displayed version before comparing its
links with the latest published documentation.
