<!-- FR: Définit les mesures reproductibles du runtime actuel. EN: Defines reproducible measurements for the current runtime. Called by: maintainers; Calls: integration fixture. -->
# Vérification des performances d’exécution

[English](BENCHMARKS.md)

Aucune mesure de capacité n’est publiée pour le moteur de protection actuel.
La réussite des tests Go natifs ou intégrés Traefik démontre un comportement,
pas un débit ni une résistance aux afflux distribués. Les temps natifs ne
représentent pas non plus l’interpréteur Yaegi utilisé par Traefik.

## Mesure reproductible

Utiliser la pile isolée `test/docker-compose.yml`, qui charge les sources locales
et désactive les échanges avec l’API en ligne CrowdSec. Ne pas générer de trafic
d’attaque contre la production ni partager des alertes synthétiques.

```bash
./scripts/ci-docker.sh compose --project-name pareflux-perf -f test/docker-compose.yml up --build --detach --wait
curl --header 'Host: pareflux.localhost' http://127.0.0.1:18080/
# Effectuer les mesures contre cette pile publiée uniquement sur loopback.
./scripts/ci-docker.sh compose --project-name pareflux-perf -f test/docker-compose.yml down --volumes
```

Comparer sur le même hôte, backend, générateur, nombre de requêtes et concurrence.
Consigner le commit source, les versions Go/Traefik/CrowdSec/cache, les limites
CPU et mémoire, la chauffe, la charge utile, le mode, les réglages AppSec et le
nombre de décisions. Mesurer séparément le trafic autorisé et bloqué, la recherche
IP/plage stream, les succès/défauts de cache live, l’inspection des corps et la
synchronisation des instantanés. Répéter chaque mesure et publier le débit, les
latences p50/p95/p99, les erreurs et le pic mémoire. Une mesure locale n’est pas
une capacité universelle. Recommencer lors d’un changement du moteur, du runtime
ou des dépendances.
