<!-- FR: Documente les licences et origines conservées. EN: Documents preserved licenses and origins. Called by: readers; Calls: source attributions. -->
# Licences tierces

[English](THIRD_PARTY_LICENSES.md)

| Composant | Version | Licence |
| --- | --- | --- |
| Sources dérivées Crowdsec Bouncer Traefik Plugin | v1.6.0 | Apache-2.0 |
| PareFlux TTL Cache | v0.1.3 | Apache-2.0 |
| Sources dérivées simpleredis | v1.0.12 | Apache-2.0 |

Les dépendances embarquées se limitent aux versions examinées dans `go.mod`,
`go.sum` et `vendor/modules.txt`. Le client simpleredis modifié réside dans
`pkg/simpleredis` pour préserver une copie reproductible des dépendances.
Le cache inclut le correctif de priorité aux expirations vérifié par condensat
dans `patches/ttl-cache-expiry.json`.
