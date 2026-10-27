<!-- FR: Documente les licences et origines conservées. EN: Documents preserved licenses and origins. Called by: readers; Calls: source attributions. -->
# Provenance des sources

[English](UPSTREAM.md)

## Sources importées

- Dépôt : `https://github.com/maxlerebourg/crowdsec-bouncer-traefik-plugin`
- Tag : `v1.6.0`
- Commit : `1f6a8991c8f9273cf21a5da3a6e6875a5c0034cc`
- Licence : Apache-2.0

Seuls certains fichiers source et tests Apache-2.0 ont été copiés depuis une
copie amont extérieure vers ce dépôt Git initialisé séparément.
Les répertoires amont `.git`, `vendor/`, `go.mod`, `go.sum`, exemples et
l’implémentation GPL-3.0 `github.com/leprosus/golang-ttl-map` ne font pas partie
des sources importées.

## Adaptations

- Identité du module et du paquet : PareFlux Gate.
- Cache GPL-3.0 remplacé par `gitlab.nikolov.fr/application/pareflux-ttl-cache`.
- Manifeste, documentation, versions, CI et attribution propres à PareFlux.
- Modifications PareFlux suivies dans ce dépôt.
- Client Apache-2.0 `github.com/maxlerebourg/simpleredis` v1.0.12 renforcé et
  intégré à `pkg/simpleredis` pour une copie reproductible des dépendances.
