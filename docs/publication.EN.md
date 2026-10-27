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
Existing tags are immutable. GitLab is the source of truth and its CI is the
only publication path to GitHub. Push the reviewed source branch, then tag that same validated commit. Adapt the
branch workflow to the repository; do not bypass branch protection. After local verification:

```bash
PUBLISH_DRY_RUN=1 PAREFLUX_GITHUB_REPOSITORY=https://github.com/vandman/pareflux-traefik-gate.git ./scripts/publish-github.sh vX.Y.Z
git push origin HEAD
git tag -a publish/vX.Y.Z -m "PareFlux Gate vX.Y.Z"
git push origin refs/tags/publish/vX.Y.Z
```

The protected tag pipeline runs Go tests and real Traefik/Yaegi integration,
then atomically publishes the export and `vX.Y.Z` tag on GitHub. The GitLab
`PAREFLUX_GITHUB_DEPLOY_KEY` variable must be a protected file variable; its
public key must have write access to the plugin GitHub repository only.
A missing key fails publication instead of silently omitting the job.
Check the tag pipeline to confirm publication. On failure, fix the cause and
retry the appropriate job; do not publish from the workstation.

The isolated export uses Git-ignored `.local/publication-work` and is removed
on exit. Git, Bash and Go or Docker are required. Local dry runs do not publish.
Never move or republish an existing tag.

## Version consistency

The source `pluginVersion` is the version being prepared, not proof that its
public GitHub tag exists. Check public tags before asking users to install it.
Development CI runs `go run -mod=vendor ./scripts/release-examples` without
writing: both guides and the Compose plugin pin must match `version.go`.

After exporting and setting `version.go` to the release version, the publisher
runs `scripts/release-examples --version vX.Y.Z --write`, then checks it again.
It updates five pins: the introductory version and YAML version in each guide,
and the Compose plugin version. Missing or duplicate pins stop publication.
Traefik, CrowdSec, Redis and dependency versions are preserved. These version
changes occur only in the export; no CI commit is pushed back to GitLab.

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
- Link preparation preserves code examples, inline code and README section links; the separate version step intentionally updates plugin pins in examples.
- Link rewriting affects only the catalog README. The separate version step also updates the French guide and Compose example.
- External websites and the GitLab development-repository link remain unchanged; their availability is not a local-file check.

A raw repository root is not a usable download URL. Use the GitHub repository
page to browse files, or specify both the version and the complete file path
for raw content. The publisher verifies all local README targets before writing the
converted README, then runs a second pass in check-only mode.

The catalog indexes GitHub asynchronously. Publishing a tag does not immediately
refresh the catalog page; verify the displayed version before comparing its
links with the latest published documentation.
