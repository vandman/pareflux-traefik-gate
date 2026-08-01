<!--
###############################################################################
# Public pull request template
# FR: Cadre la verification des contributions publiques avant leur integration
#     dans le depot GitLab principal et leur prochaine publication stable.
# EN: Frames public contribution verification before integration into the
#     primary GitLab repository and the next stable publication.
# Called by: GitHub pull request creation
# Calls: GitHub Actions and PareFlux maintainer review
###############################################################################
-->

## Purpose / Objectif

<!-- EN: Explain the problem and why this change is appropriate. -->
<!-- FR : Expliquez le probleme et pourquoi cette modification est adaptee. -->

## Changes / Modifications

<!-- Describe the behavior changed by this pull request. -->

## Validation

- [ ] `gofmt` produces no changes.
- [ ] Go vet, race tests, and license checks pass.
- [ ] The real Traefik/CrowdSec Compose test passes when runtime behavior changes.
- [ ] New or changed behavior has tests.
- [ ] Documentation is updated in English and French when applicable.
- [ ] No secret, credential, private address, or identifying data is included.
- [ ] Third-party code and dependencies are Apache-2.0-compatible and attributed.

## Security impact / Impact securite

<!-- Cover trusted client identity, cache state, and fail-safe behavior where relevant. -->

## Integration note / Note d'integration

GitLab is the primary development repository. An accepted public contribution
is reviewed and integrated there before appearing in a later stable GitHub
release.

GitLab est le depot de developpement principal. Une contribution publique
acceptee y est revue et integree avant une publication stable GitHub ulterieure.
