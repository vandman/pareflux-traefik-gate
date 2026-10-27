<!-- FR: Explique les limites et preuves du moteur. EN: Explains engine limits and evidence. Called by: operators; Calls: configuration reference. -->
# Moteur de protection et politiques de panne

[English](security-runtime.EN.md) · [Configuration](configuration.FR.md)

## Cycle de vie du moteur et décisions

Gate a besoin de CrowdSec ; Control est la console facultative. Les configurations
amont résolues identiques partagent un moteur, ses tâches de synchronisation et
son cache. Une autre clef ou un autre certificat crée un moteur distinct. La
libération du dernier contexte de configuration Traefik annule les tâches et
appels HTTP. Les tests intégrés de rechargement réel exercent ce cycle de vie.

Le moteur stream effectue une première synchronisation avant d’annoncer un état
sain. Chaque interrogation demande un instantané complet pour éviter de perdre
des changements pendant un rechargement ou entre consommateurs. L’intervalle par
défaut est de 60 secondes ; le réduire augmente le travail de LAPI et de
Traefik/Yaegi. `alone` interroge l’API centrale toutes les deux heures et utilise
le même magasin de décisions.

Les décisions actives sont indexées par identifiant et préfixe IPv4/IPv6. Un
blocage est prioritaire sur un captcha, même avec des plages superposées.
Supprimer un identifiant ne supprime pas un autre blocage applicable. Les
adresses, durées et portées non prises en charge sont ignorées et comptées dans
les journaux et `skipped_decisions`.

Le magasin n’évince jamais de blocage actif. `streamDecisionCapacity` vaut
262144 par défaut (maximum 1000000). Sa saturation conserve les blocages connus
et **refuse immédiatement les clients inconnus**, avec une erreur explicite,
jusqu’à ce qu’un instantané complet tienne dans la limite. Ajouter des abonnements
à des listes demande donc de surveiller leur taille et d’adapter la capacité.
Il n’existe pas d’option autorisant les inconnus en cas de saturation.

Les instantanés sont décodés entrée par entrée ; une troncature, une clef racine
dupliquée ou une valeur excessive ne remplace jamais les décisions actives.
Les opérations JSON sont limitées à 64 Kio plus la lecture anticipée native.
La réponse est plafonnée à 512 octets par emplacement configuré plus 1 Mio,
avec un minimum de 16 Mio et un maximum de 256 Mio. Une réponse exceptionnellement
volumineuse reste une erreur de synchronisation même si le nombre de décisions tient.

## Pannes des services amont

Un refus pendant une panne amont protège le trafic lorsque Gate ne peut plus le
vérifier ; cela ne signifie pas nécessairement que Gate lui-même est tombé en panne.

| Situation | Comportement par défaut |
| --- | --- |
| Aucun succès de synchronisation depuis le démarrage | Refuser les clients inconnus |
| Panne stream après une synchronisation réussie | Conserver les blocages non expirés ; refuser les inconnus seulement après **à la fois** plus de 3 échecs consécutifs et plus de 60 secondes depuis le dernier succès |
| `updateMaxFailure: -1` | Désactiver la transition liée au compteur ; la grâce écoulée seule ne rend pas bloquant un flux auparavant sain |
| Échec de connexion AppSec ou HTTP 502/503/504 | Après inspection réussie, tolérer tant que le dernier succès date d’au plus 60 secondes **ou** que le compteur ne dépasse pas 3 ; refuser après dépassement des deux |
| Expiration d’inspection AppSec, HTTP 500 ou refus de sécurité | Aucune grâce de panne ; refuser |
| Échec d’interrogation LAPI en live/none | Refuser |

`failureGraceSeconds` et `updateMaxFailure` modifient ces défauts. Mettre
`updateMaxFailure: -1` maintient aussi les erreurs AppSec admissibles dans la
grâce après un premier succès. `crowdsecStreamUnreachableBlock: false` autorise
les clients inconnus pendant une panne du flux.
`crowdsecAppsecUnreachableBlock: false` autorise les échecs d’établissement de
connexion ; `crowdsecAppsecFailureBlock: false` autorise les HTTP 502/503/504.
Aucun des deux n’autorise les expirations d’inspection, HTTP 500 ou attaques.
Le code configurable des blocages est distinct des erreurs techniques 408/413/503.

## Inspection AppSec

| Limite | Défaut / plafond |
| --- | --- |
| Préfixe de corps inspecté | 1 Mio ; configurable jusqu’à 16 Mio |
| Corps trop volumineux | HTTP 413 sauf inspection partielle explicitement configurée |
| Budget lecture du corps et inspection | 3 secondes ; annulation cliente propagée |
| Mémoire commune des tampons | 64 Mio par processus Traefik |
| Lecteurs de corps simultanés | 1024 par processus Traefik |
| Corps préchargés par client | 4 par adresse IPv4 ou IPv6 /64 ; quota IPv6 /48 quatre fois supérieur |
| Enregistrements de quota client | Au plus 4096 ; entrées inactives supprimées |
| Délai de lecture après inspection | 60 secondes depuis l’entrée dans Gate ; aligner avec `respondingTimeouts.readTimeout` de Traefik |

Les requêtes sans corps restent inspectées sur les métadonnées sans consommer
le quota de corps. Les en-têtes MIME/Upgrade gRPC ou WebSocket n’exemptent pas
un corps. Les cookies et `Authorization` ne sont volontairement pas transmis :
les règles qui examinent leur contenu, notamment les règles CRS sur les cookies,
ne peuvent pas inspecter ces champs.

Un seul tampon de restitution utilise des blocs de 8 Kio réservés progressivement,
jusqu’à la limite plus un octet de détection. Il est libéré lorsque les écrivains
AppSec et de restitution au backend ont fini de le lire, sans attendre la réponse
applicative. Quatre corps parallèles partagent le quota derrière un NAT/CGNAT
IPv4 ; adapter `crowdsecAppsecMaxConcurrentPerClient` aux envois légitimes en
tenant compte des limites globales.

L’expiration d’inspection rend la main au gestionnaire, mais un lecteur enveloppé
non interruptible conserve sa place et son quota jusqu’à sa fin réelle. Garder
un **délai de lecture fini sur l’entrypoint Traefik**, par exemple 60 secondes.
Gate restaure un délai fini si possible, jamais nul ;
`crowdsecAppsecReadTimeoutSeconds: 0` laisse les délais de Traefik inchangés.

Le nombre de lecteurs et le budget mémoire sont deux bornes distinctes. Des
clients distribués peuvent encore saturer les lecteurs (256 adresses IPv4 ou
64 allocations IPv6 /48 à quatre corps par client), éventuellement jusqu’au
délai de l’entrypoint. Ces limites bornent les ressources ; elles ne garantissent
pas la disponibilité face à un afflux distribué.

Garder le refus sur les routes sensibles. Déclarer l’inspection partielle pour
des hôtes d’envoi, chemins ancrés et méthodes précis, par exemple :

```yaml
crowdsecAppsecBodyExceptions:
  - host: registry.example.org
    pathRegexp: '^/v2/.*/blobs/uploads(?:/.*)?$'
    methods: [POST, PATCH, PUT]
```

Au plus 32 exceptions sont acceptées. Elles conservent le contrôle IP, les
métadonnées et le préfixe inspecté ; le reste **n’est pas inspecté**. Les réponses
partielles portent `X-Pareflux-Appsec-Inspection: partial` et ne confirment pas
une inspection complète dans le dashboard. `crowdsecAppsecBodyLimitAction: partial`
s’applique à tout le middleware : le réserver aux routes d’envoi dédiées. Une
limite de corps nulle n’inspecte que les métadonnées.
`crowdsecAppsecBodyExcludedContentTypes` vaut `[]` et est ignoré ; il ne contourne
pas l’inspection. Les politiques identiques d’entrypoint et de routeur sont exécutées une fois.

## Cache et administration

TTL/Redis accélère seulement les consultations live et les captchas. Chaque
configuration utilise un espace Redis versionné distinct ; une éviction Redis
ne retire pas de blocage stream. Le cache TTL embarqué utilise un correctif vérifié
par condensat sur v0.1.3 : avant éviction FIFO, il examine au plus 64 positions
pour trouver des entrées expirées. Au-delà, une entrée valide peut encore être
évincée. Après `go mod vendor`, exécuter `./scripts/patch-ttl-cache-vendor.sh` ;
il refuse une source inattendue et est idempotent.

Avant de rendre les pannes bloquantes, prévoir si nécessaire une adresse de
gestion vérifiée et précisément limitée dans `clientTrustedIps`. Elle contourne
**tous** les contrôles, AppSec compris. `forwardedHeadersTrustedIps` permet
seulement à un véritable proxy HTTP amont de fournir l’identité du visiteur ;
cela n’exempte pas ses visiteurs. Un routeur NAT n’est pas un tel proxy. Ne
jamais approuver indistinctement des réseaux publics.

## Preuves du dashboard et découverte

Le dashboard distingue la connexion à Control, la synchronisation des décisions
et l’inspection AppSec de cette requête. Un ancien moteur sain ne confirme pas
un nouveau moteur. `range_decisions` signé concerne seulement le moteur du menu
privé correspondant : références actives, synchronisation récente, aucune erreur
ni saturation. Copier les options de moteur du middleware de protection réel
dans les instances de navigation.

La découverte exige une clef partagée d’au moins 32 octets et une route privée
d’enregistrement réservée à Control. Les requêtes et réponses sont signées par
HMAC, horodatées et liées à un nonce ; les annonces rejouées ou expirées sont
refusées. Les liens expirent après 90 secondes sans renouvellement même si une
ancienne sonde a réussi. Aucun cookie utilisateur ni identifiant SSO n’est échangé.
Voir la [configuration complète du dashboard](dashboard-menu.FR.md).
