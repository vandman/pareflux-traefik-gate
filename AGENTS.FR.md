<!-- FR: Traduit les instructions locales pour les contributeurs. EN: Translates local contributor instructions. Called by: contributors; Calls: AGENTS.md. -->
# AGENTS — PareFlux Traefik Gate

[English et instructions utilisées par les agents](AGENTS.md)

## Portée

Ces instructions concernent ce dépôt.

## Règles

- Conserver les sources originales et dérivées sous Apache-2.0.
- Préserver les attributions amont et les mentions de fichiers modifiés.
- Ne jamais ajouter de code GPL-3.0, AGPL, de licence inconnue ou de dépendance embarquée non examinée.
- Garder le plugin local en lecture seule pendant son exécution.
- Conserver les décisions persistantes dans la LAPI CrowdSec, pas dans le cache du plugin.
- Distinguer le pair réseau des en-têtes d’identité transférés.
- Vérifier chaque publication avec les tests Go natifs et de vrais tests Traefik/Yaegi.
- Garder les documentations française et anglaise synchronisées.
- Appliquer les commentaires bilingues du workspace aux nouveaux fichiers et fonctions.
