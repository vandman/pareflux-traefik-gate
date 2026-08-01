# Benchmarks d'exécution

## Comparaison Traefik/Yaegi

Le premier benchmark PareFlux Gate a comparé :

- Traefik `v3.7.5` ;
- le plugin amont `v1.6.0` au commit
  `1f6a8991c8f9273cf21a5da3a6e6875a5c0034cc` ;
- PareFlux Gate utilisant PareFlux TTL Cache `v0.1.0` ;
- un chemin autorisé en mode live avec cache chaud ;
- 50 000 requêtes par exécution, concurrence 32, trois exécutions ;
- le même hôte, backend, faux LAPI, images et générateur de charge.

Le checkout amont et le cache GPL sont restés hors de ce dépôt.

| Implémentation | Requêtes/s | p50 | p95 | p99 | Erreurs |
| --- | ---: | ---: | ---: | ---: | ---: |
| Cache amont | 3 427 | 7,966 ms | 20,271 ms | 28,663 ms | 0 |
| Cache PareFlux | 3 365 | 8,132 ms | 20,516 ms | 29,452 ms | 0 |

La différence de débit mesurée est d'environ `-1,8 %`, dans les tolérances
prédéclarées de 5 % en natif et 10 % dans le runtime réel. Ce benchmark local
ne constitue pas une affirmation universelle de capacité.

## Résultat natif du cache

Sur le même hôte, le chemin `Get` chaud du cache PareFlux mesure environ
19–20 ns/op sans allocation. La référence amont externe mesure environ
24–25 ns/op sans allocation. Le `Set` PareFlux mesure environ 80 ns/op contre
environ 82–90 ns/op pour l'amont.

Les benchmarks doivent être relancés à chaque changement de version de
Traefik, Go, Yaegi ou du cache.
