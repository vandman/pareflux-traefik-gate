<!--
###############################################################################
# Public contribution guide
# FR: Explique le parcours GitHub vers GitLab imposé par le catalogue Traefik.
# EN: Explains the GitHub-to-GitLab contribution path required by the catalog.
# Called by: PareFlux Gate contributors
# Calls: GitHub pull requests and the primary GitLab promotion workflow
###############################################################################
-->

# Contributing to PareFlux Gate

## Why GitLab and GitHub are both used

PareFlux Gate is developed on the project's GitLab instance. GitLab is the
primary development repository and contains intermediate commits, testing work,
and development releases.

GitHub is the public distribution and contribution channel because the Traefik
Plugin Catalog only accepts plugins hosted in public GitHub repositories. Only
versions considered sufficiently stable are promoted from GitLab to GitHub, so
the GitHub history does not contain every intermediate development commit or
version.

## Pull request process

Public issues and pull requests are welcome on GitHub. A proposed patch is first
reviewed and tested without secrets. When accepted, it is integrated and
validated in the primary GitLab repository, then included in a subsequent
stable GitHub release.

Please keep the pull request open while this integration is in progress. A
merged pull request may not be available in a catalog release immediately.

By contributing, you agree that your contribution is licensed under
Apache-2.0, the license used by this project.
