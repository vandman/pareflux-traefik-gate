# PareFlux Gate

**Reliez Traefik à CrowdSec pour bloquer les accès malveillants à vos applications web.**

> **Le plugin seul ne constitue pas un système de protection complet.** PareFlux
> Gate **ne contient pas CrowdSec**. Dans l’installation standard, un service
> CrowdSec distinct doit être disponible, déjà installé ou déployé avec Traefik.
> **La console PareFlux est facultative ; la source de protection CrowdSec ne l’est pas.**

[English](https://github.com/vandman/pareflux-traefik-gate/blob/main/README.md) · **Français** · [Catalogue Traefik](https://plugins.traefik.io/plugins/6a6da8539dbe8a37899c23c8/pare-flux-gate)

PareFlux Gate s’exécute dans Traefik, le service qui reçoit les connexions à vos
applications web. CrowdSec fonctionne séparément : il repère les activités
suspectes et fournit des consignes de blocage, par exemple refuser une adresse IP
pendant une heure. PareFlux Gate applique ces consignes aux connexions entrantes.

CrowdSec peut aussi examiner la requête web en cours pour reconnaître une
attaque. Cette fonction facultative de CrowdSec s’appelle **AppSec** ; elle n’est
pas incluse dans le plugin. La **console PareFlux (PareFlux Control)** est une
interface séparée et facultative pour consulter et administrer la protection.

| Composant | Nécessaire ? | Fourni dans le plugin ? |
| --- | --- | --- |
| Traefik, qui reçoit les connexions aux applications | Oui | Non |
| PareFlux Gate, qui applique la protection dans Traefik | Oui | C’est le plugin |
| CrowdSec, qui fournit les consignes de blocage | Oui, comme source de protection | **Non, il fonctionne séparément** |
| AppSec, la fonction CrowdSec qui examine les requêtes web | Facultatif | Non |
| Console PareFlux, pour consulter et administrer la protection | Facultative | Non |

L’exemple Compose complet installe Traefik avec PareFlux Gate et un service
CrowdSec distinct avec AppSec activé. **L’exemple assemble ces services ;
CrowdSec n’est pas embarqué dans le plugin.**

## Sommaire

- [Fonctionnalités](#fonctionnalités)
- [Choisir un mode](#choisir-un-mode)
- [Installation](#installation)
- [Vérifier la protection](#vérifier-la-protection)
- [Réglages et politiques de panne](#réglages-et-politiques-de-panne)
- [Dépannage](#dépannage)
- [Architecture](#architecture)

## Fonctionnalités

| Besoin | Ce que fait Gate |
| --- | --- |
| Appliquer les consignes de blocage | Refuser une IP ou présenter un captcha lorsque CrowdSec le demande |
| Examiner les requêtes web | Demander, en option, à CrowdSec AppSec de détecter les requêtes malveillantes avant transmission |
| Éviter une interrogation de CrowdSec par visiteur | Synchroniser les consignes en mémoire ou conserver temporairement les résultats individuels |
| Identifier le visiteur derrière un proxy | En-tête transféré accepté uniquement depuis un proxy explicitement approuvé |
| Adapter les refus | Code HTTP, page HTML de blocage, fournisseur et page captcha configurables |
| Diagnostiquer les blocages | Journaux et marqueur de remédiation configurable dans la réponse et la requête interne |

Une **décision** est une consigne de blocage avec une cible (IP ou plage réseau)
et une expiration. **LAPI** désigne l’API locale de CrowdSec : le service que
Gate interroge pour obtenir ces consignes, généralement sur le port 8080. Un
**bouncer** est un composant qui les applique ; Gate en est un. **TTL** désigne
la durée de conservation d’une entrée temporaire. Redis est un service de cache
externe facultatif.

CrowdSec analyse les journaux et produit les décisions. Gate les applique ; il
n’installe pas de scénarios ni ne remplace le moteur CrowdSec. Les décisions
persistantes restent dans LAPI ; le cache local est perdu au redémarrage.
Le flux communautaire facultatif fournit des consignes CrowdSec pour des
menaces observées ailleurs. Son accès dépend de l’inscription et des flux activés
pour ce compte CrowdSec ; il n’est pas nécessaire à la détection locale.

## Choisir un mode

| Mode | Source des décisions | Cache | Usage |
| --- | --- | --- | --- |
| `stream` | CrowdSec LAPI, instantané complet toutes les 60 secondes par défaut | Magasin IP/plages en mémoire, sans éviction des blocages actifs | Point de départ conseillé pour appliquer les décisions LAPI avec peu de travail par requête |
| `live` | LAPI si l’IP manque dans le cache | Local ou Redis ; résultat conservé au plus 60 secondes par défaut | Interroger les IP à leur arrivée |
| `none` | LAPI à chaque requête | Pas de cache de consultation des décisions IP | Exiger une réponse LAPI pour chaque requête ; ce mode ne **désactive pas** la protection |
| `appsec` | CrowdSec AppSec uniquement | Pas de consultation des décisions IP | Inspecter HTTP lorsque les décisions IP sont déjà appliquées ailleurs ; définir `crowdsecAppsecEnabled: true` |
| `alone` | API centrale CrowdSec, toutes les deux heures | Même magasin IP/plages que stream | Consommer le flux communautaire avec des identifiants CAPI valides ; n’inclut pas les décisions LAPI locales |

AppSec peut compléter les modes de décisions IP en définissant
`crowdsecAppsecEnabled: true`. Le mode par défaut est `live`, mais l’exemple
ci-dessous choisit explicitement `stream`.

## Installation

L’exemple public utilise la version **v0.1.24**. La configuration statique
charge le plugin au démarrage ; la configuration dynamique l’active et le
rattache aux routes à protéger. Déclarer le plugin seul ne protège aucune route.

### 1. Préparer CrowdSec

Gate se connecte **directement à CrowdSec**, sans passer par la console PareFlux.
Utiliser l’adresse de **l’API locale CrowdSec**, pas celle de son site web ni de
la console. Par exemple, `crowdsecLapiHost: crowdsec:8080` suppose que Traefik
résout le service `crowdsec` et atteint son port 8080 sur un réseau privé partagé.
Pour un moteur distant, indiquer son nom privé joignable et configurer HTTPS.
Ne pas exposer cette API publiquement.

Sur l’hôte CrowdSec, créer une clef dédiée :

```bash
sudo cscli bouncers add pareflux-traefik
# Avec Docker : docker compose exec crowdsec cscli bouncers add pareflux-traefik
```

La commande affiche une clef API secrète. La conserver en lieu sûr et la monter en lecture seule **dans le conteneur Traefik** sous `/run/secrets/crowdsec-bouncer.key`.
Le compte exécutant Traefik doit pouvoir la lire. Pour un service Compose
existant, enregistrer uniquement la clef affichée dans
`./secrets/crowdsec-bouncer.key` (hors Git), puis ajouter ce montage :
```yaml
services:
  traefik:
    volumes:
      - ./secrets/crowdsec-bouncer.key:/run/secrets/crowdsec-bouncer.key:ro
```


Pour AppSec, CrowdSec doit aussi disposer d’un composant AppSec en écoute sur
`:7422`, de ses acquisitions et règles. Sinon, commencer avec
`crowdsecAppsecEnabled: false`. Pour détecter des comportements à partir des
journaux, configurer séparément l’acquisition des logs Traefik dans CrowdSec.

### 2. Déclarer le plugin dans Traefik

Dans le fichier **statique** `traefik.yml` :

```yaml
experimental:
  plugins:
    parefluxgate:
      moduleName: github.com/vandman/pareflux-traefik-gate
      version: v0.1.24
```

Redémarrer Traefik après un changement de version du plugin. Utiliser le
fournisseur de fichiers existant, ou y déclarer le fichier dynamique ci-dessous :

```yaml
providers:
  file:
    filename: /etc/traefik/dynamic/pareflux.yml
```

### 3. Créer le middleware et l’attacher

Dans `/etc/traefik/dynamic/pareflux.yml`, en adaptant le domaine, le service et
les adresses CrowdSec à l’installation :

```yaml
http:
  routers:
    app:
      rule: Host(`app.example.com`)
      entryPoints: [websecure]
      tls: {}
      service: app
      middlewares: [pareflux-gate]
  services:
    app:
      loadBalancer:
        servers:
          - url: http://app:80
  middlewares:
    pareflux-gate:
      plugin:
        parefluxgate:
          enabled: true
          crowdsecMode: stream
          crowdsecLapiHost: crowdsec:8080
          crowdsecLapiKeyFile: /run/secrets/crowdsec-bouncer.key
          crowdsecAppsecEnabled: false
          remediationHeadersCustomName: X-Pareflux-Remediation
```

Cet exemple suppose que l’entrypoint `websecure` existe et que le certificat TLS
est géré par la configuration Traefik habituelle. Conserver les middlewares de
SSO et autres contrôles déjà présents. Pour référencer ce middleware depuis
un routeur Docker, utiliser `pareflux-gate@file`.

### Variante : labels Docker

Si le routeur et son middleware sont définis par le fournisseur Docker,
utiliser cette variante **à la place** du middleware défini dans le fichier :

```yaml
services:
  app:
    labels:
      traefik.http.routers.app.middlewares: pareflux-gate
      traefik.http.middlewares.pareflux-gate.plugin.parefluxgate.enabled: "true"
      traefik.http.middlewares.pareflux-gate.plugin.parefluxgate.crowdsecMode: stream
      traefik.http.middlewares.pareflux-gate.plugin.parefluxgate.crowdsecLapiHost: crowdsec:8080
      traefik.http.middlewares.pareflux-gate.plugin.parefluxgate.crowdsecLapiKeyFile: /run/secrets/crowdsec-bouncer.key
      traefik.http.middlewares.pareflux-gate.plugin.parefluxgate.crowdsecAppsecEnabled: "false"
```

Compléter avec les labels habituels du routeur, de l’entrypoint et du service.
La clef est toujours un fichier monté dans Traefik, jamais un secret dans les
labels. Pour protéger plusieurs applications, rattacher le même middleware à
leurs routeurs.

### En option : examiner les requêtes avec AppSec

Faire d’abord fonctionner le blocage IP. Activer ensuite le composant AppSec
distinct de CrowdSec sur le port 7422, avec ses acquisitions et collections de
règles. L’[exemple d’acquisition AppSec](examples/compose/appsec.yaml) et le
réglage `COLLECTIONS` de Compose montrent ce montage. Voir aussi le
[guide AppSec CrowdSec](https://docs.crowdsec.net/docs/appsec/quickstart/traefik/).
Ne pas reprendre le nom de module ou la version d’un autre plugin de ce guide.

Ajouter ces options au **middleware de protection existant** :

```yaml
crowdsecAppsecEnabled: true
crowdsecAppsecHost: crowdsec:7422
crowdsecAppsecBodyLimit: 1048576
```

La clef AppSec hérite de la clef LAPI dans les modes LAPI sauf remplacement
explicite. AppSec examine les métadonnées et le corps ; les cookies et
`Authorization` sont volontairement exclus. Un corps dépassant **1 Mio est
refusé par défaut avec HTTP 413**. Prévoir des
[exceptions explicites d’envoi](docs/security-runtime.FR.md#inspection-appsec)
avant activation sur les applications acceptant de gros fichiers. Le budget
d’inspection est de 3 secondes ; garder un `readTimeout` fini sur l’entrypoint
Traefik, par exemple 60 secondes, cohérent avec `crowdsecAppsecReadTimeoutSeconds`.

### Démonstration complète

L’[exemple Compose](https://github.com/vandman/pareflux-traefik-gate/tree/main/examples/compose)
contient Traefik, CrowdSec, AppSec et Whoami. Récupérer une fois le dépôt public,
puis lancer la démonstration :

```bash
git clone https://github.com/vandman/pareflux-traefik-gate.git
cd pareflux-traefik-gate/examples/compose
docker compose up --detach --wait
curl --header 'Host: pareflux.localhost' http://127.0.0.1:18080/
# Supprimer uniquement cette pile de démonstration et ses données :
docker compose down --volumes
```

La démonstration active explicitement AppSec et désactive les échanges avec
l’API en ligne CrowdSec (`DISABLE_ONLINE_API: "true"`) pour ne pas partager le
trafic de test. Elle ne configure pas l’acquisition des journaux d’accès Traefik :
la détection automatique dans les journaux demande ce réglage supplémentaire.
La synchronisation toutes les secondes sert à la démonstration ; commencer
avec le défaut de 60 secondes en production.

Cette démonstration écoute uniquement sur la boucle locale. Sa clef intégrée
est jetable ; elle n’est pas destinée à la production. En production, utiliser
une clef dédiée et préférer un proxy de socket Docker au montage direct du socket.

## Vérifier la protection

1. Vérifier qu’une requête normale atteint l’application.
2. Sur une installation de test, ajouter une décision temporaire pour l’IP
   **réellement vue par Gate**, puis répéter la requête depuis cette IP.
3. Attendre la synchronisation en mode `stream` ; constater le refus, puis
   supprimer la décision et vérifier le retour à la normale.

```bash
# Sur le moteur CrowdSec de TEST ; IP réservée à la documentation :
cscli decisions add --ip 192.0.2.123 --duration 2m --reason pareflux-test
cscli decisions delete --ip 192.0.2.123
```

Remplacer `192.0.2.123` par l’IP du client de test. Ne pas bannir l’IP utilisée
pour administrer l’installation. Un test de décision IP ne prouve pas à lui
seul que les acquisitions de logs ou les règles AppSec fonctionnent.

## Réglages et politiques de panne

| Option | Défaut | Rôle |
| --- | --- | --- |
| `enabled` | `false` | Activer explicitement le middleware de protection |
| `crowdsecMode` | `live` | Choisir la source et la fréquence des décisions |
| `crowdsecLapiKeyFile` | Vide | Lire la clef LAPI depuis un secret monté |
| `updateIntervalSeconds` | `60` | Fréquence du flux LAPI en mode `stream` |
| `defaultDecisionSeconds` | `60` | Durée maximale du résultat en cache en mode `live` |
| `crowdsecAppsecEnabled` | `false` | Activer l’inspection HTTP |
| `remediationStatusCode` | `403` | Code de refus d’un ban |
| `forwardedHeadersTrustedIps` | `[]` | Proxies autorisés à transmettre l’IP du visiteur |
| `clientTrustedIps` | `[]` | Clients exemptés de **tous** les contrôles Gate |

La [référence complète des options](https://github.com/vandman/pareflux-traefik-gate/blob/main/docs/configuration.FR.md)
précise les types, valeurs par défaut, fichiers secrets, TLS, Redis et captcha.
Les noms sont ceux du code PareFlux : une option présente dans un autre fork
CrowdSec n’est pas automatiquement disponible ici.

| Panne | Politique par défaut | Réglage |
| --- | --- | --- |
| Flux LAPI/CAPI déclaré en échec | Après le seuil d’échecs et la grâce, refuser les IP sans décision connue ; les blocages non expirés restent appliqués | `crowdsecStreamUnreachableBlock: true` |
| Échec d’établissement de connexion AppSec | Refuser après la grâce de panne admissible | `crowdsecAppsecUnreachableBlock: true` |
| AppSec HTTP 502/503/504 | Refuser après la grâce de panne admissible | `crowdsecAppsecFailureBlock: true` |
| Consultation Redis nécessaire mais indisponible | Refuser la requête nécessitant cette consultation | `redisCacheUnreachableBlock: true` |

Passer une de ces options à `false` privilégie la disponibilité pour ce cas
précis. Les modes `live` et `none` refusent aussi la requête si leur interrogation
LAPI échoue. Le flux tolère par défaut 3 échecs consécutifs et une grâce de 60 secondes,
après une première synchronisation réussie. Sans celle-ci, le démarrage reste
bloquant. La grâce AppSec ne couvre que les échecs de connexion et les HTTP
502/503/504 après inspection réussie ; les expirations d’inspection et HTTP 500
restent bloquants. Dépasser `streamDecisionCapacity` refuse immédiatement les
clients inconnus. Voir les [seuils et limites précis](docs/security-runtime.FR.md).

Les proxies approuvés et les clients exemptés ont des rôles différents :
`forwardedHeadersTrustedIps` permet de lire l’IP d’origine ; `clientTrustedIps`
contourne la protection pour l’IP résolue. Ne pas y placer indistinctement des
réseaux publics. Un routeur NAT ne fournit pas d’en-têtes d’identité HTTP. Les
routes ayant la même configuration CrowdSec résolue partagent un moteur ; une
adresse, des identifiants ou des options de moteur différents créent un autre moteur.

## Dépannage

| Symptôme | Vérification |
| --- | --- |
| Aucune requête n’est filtrée | `enabled: true`, middleware attaché au bon routeur et bon fournisseur (`@file` / `@docker`) |
| Mauvaise IP ou tous les visiteurs bloqués | Adresse du pair, chaîne de proxies et `forwardedHeadersTrustedIps` ; ne pas faire confiance à tous les en-têtes |
| Refus après une panne de CrowdSec | Journaux Gate, accès réseau LAPI/AppSec et politiques de panne ci-dessus |
| Erreur LAPI `401` / `403` | Clef de bouncer, fichier lisible depuis Traefik et moteur CrowdSec correspondant |
| Ban non appliqué immédiatement | Mode sélectionné, délai de flux ou cache `live`, IP réellement résolue |
| LAPI fonctionne mais AppSec refuse tout | Service `:7422`, acquisitions et règles AppSec, clef et journaux AppSec |
| Logs montrant les rejets comme de nouvelles attaques | Conserver le marqueur de remédiation dans le log d’accès et configurer son exclusion dans le traitement CrowdSec |

## Architecture

Les schémas et explications sont dans le document
[Architecture de PareFlux Gate](https://github.com/vandman/pareflux-traefik-gate/blob/main/docs/architecture.FR.md) :
composants, flux réseau, traitement HTTP et décisions en cas de panne.

## Console PareFlux et dashboard Traefik

Le plugin de protection peut fonctionner sans PareFlux Control. L’intégration
facultative au dashboard ajoute un lien PareFlux, une tuile « Connecté / Protégé »
et un échange privé d’URL avec Control. Voir la [configuration dédiée](https://github.com/vandman/pareflux-traefik-gate/blob/main/docs/dashboard-menu.FR.md).
Les URL proviennent du déploiement ; Control en mode autonome n’affiche aucun
lien de retour vers un ingress.

## Choix du stockage : local, Redis, PostgreSQL ou MariaDB

PostgreSQL et MariaDB sont bien pris en charge, mais leur configuration se fait
sur CrowdSec LAPI et non sur le middleware PareFlux. Cette séparation clarifie
le rôle de chaque stockage :

| Stockage | Utilisé par | Rôle |
| --- | --- | --- |
| Magasin de décisions en mémoire | PareFlux Gate | Décisions IP et plages stream/alone actives, sans éviction |
| Cache TTL en mémoire | PareFlux Gate | Résultats temporaires des consultations live et captchas par moteur |
| Redis | PareFlux Gate | Cache live/captcha partagé entre configurations identiques ; ne stocke pas les blocages stream |
| SQLite | CrowdSec LAPI | Stockage persistant par défaut pour une installation simple sur un nœud |
| PostgreSQL ou MariaDB | CrowdSec LAPI | Stockage LAPI persistant adapté aux installations plus importantes ou répliquées |

Changer la base CrowdSec ne modifie pas les labels PareFlux : Gate continue de
se connecter à `crowdsec:8080` et CrowdSec possède la connexion à la base.

Les commandes ci-dessous fusionnent deux fichiers Compose. Le fichier
`docker-compose.yml` fournit Traefik, Whoami, CrowdSec et AppSec ; le fichier
explicitement nommé `compose.override.*` ajoute la base choisie et configure
CrowdSec pour l'utiliser.

Pour un cache partagé Redis, utiliser la surcharge Redis. Elle choisit `live`
pour utiliser réellement le cache lors des consultations individuelles ; Redis
ne remplace pas le magasin stream. Le fichier de mot de passe fourni est jetable.
Redis n’expose aucun port sur l’hôte.

```bash
cd examples/compose
docker compose -f docker-compose.yml -f compose.override.redis.yml up --detach --wait
curl --header 'Host: pareflux.localhost' http://127.0.0.1:18080/
docker compose -f docker-compose.yml -f compose.override.redis.yml down --volumes
```

Redis est indépendant de Traefik, mais cette surcharge configure son utilisation
par Gate. Ce client utilise TCP sans TLS ; le garder sur un réseau privé de confiance.

Pour lancer l'exemple complet avec PostgreSQL :

```bash
cd examples/compose
docker compose \
  -f docker-compose.yml \
  -f compose.override.postgresql.yml \
  up --detach --wait
curl --header 'Host: pareflux.localhost' http://127.0.0.1:18080/
docker compose \
  -f docker-compose.yml \
  -f compose.override.postgresql.yml \
  down --volumes
```

Ou pour lancer la même pile avec MariaDB :

```bash
cd examples/compose
docker compose \
  -f docker-compose.yml \
  -f compose.override.mariadb.yml \
  up --detach --wait
curl --header 'Host: pareflux.localhost' http://127.0.0.1:18080/
docker compose \
  -f docker-compose.yml \
  -f compose.override.mariadb.yml \
  down --volumes
```

Les mots de passe inclus dans ces exemples sont des valeurs jetables. En
production, utilisez des secrets Docker ou un autre gestionnaire de secrets,
TLS vers une base externe et des sauvegardes durables. Le changement de base
d'une installation CrowdSec existante ne migre pas automatiquement ses
machines, clefs de remédiation, alertes ou décisions existantes.

## Vérification

```bash
go test -race ./...
go mod vendor
./scripts/patch-ttl-cache-vendor.sh
```

Chaque version doit aussi être chargée comme plugin local dans l'image Traefik épinglée et passer les tests intégrés de requêtes, blocages et rechargements.
Voir la [méthode de mesure](BENCHMARKS.FR.md) pour les performances.

## Dépôts et versions publiques

PareFlux est maintenu par un auteur français qui souhaite disposer d'une
documentation complète et de premier niveau en français. Une version anglaise
complète est maintenue en parallèle, car l'anglais est la langue internationale
de fait pour la collaboration technique et l'écosystème Traefik.

Le [dépôt GitLab](https://gitlab.nikolov.fr/application/pareflux-traefik-gate)
est le dépôt de développement principal et contient les commits intermédiaires
et les versions de développement. Le [dépôt GitHub](https://github.com/vandman/pareflux-traefik-gate)
est le canal public de distribution et de contribution, car le catalogue de
plugins Traefik accepte uniquement les dépôts GitHub publics.

Seuls des instantanés stables sont promus vers GitHub. Un tag GitLab protégé
nommé `publish/vX.Y.Z` crée un unique commit public et le tag GitHub immuable
`vX.Y.Z`. Les issues et pull requests publiques sont les bienvenues ; leur
parcours d'intégration est décrit dans
[CONTRIBUTING.FR.md](CONTRIBUTING.FR.md).

Le [guide de publication du catalogue](docs/publication.FR.md) décrit la
préparation automatique des liens du README et les contrôles de publication.

## Licence

Dérivé de l’amont Apache-2.0 v1.6.0 ; les attributions et modifications sont décrites dans [UPSTREAM.md](https://github.com/vandman/pareflux-traefik-gate/blob/main/UPSTREAM.md). PareFlux utilise son propre cache TTL Apache-2.0.

Apache-2.0. Les dépendances GPL-3.0, AGPL et de licence inconnue sont refusées.

## Contrat de confiance de la CI

Les branches et merge requests s’exécutent sans secret permanent de publication.
Une branche source terminant par `-nr` ne crée pas de pipeline. Les tests Docker
utilisent `vm-runner` et le daemon de sa VM jetable ; les autres contrôles utilisent
le runner conteneur. Seuls les tags protégés peuvent recevoir les credentials de
publication. Les tags sont créés depuis le poste opérateur ; le push Git par job
token est désactivé. Déploiements, administration de l’identité et maintenance
des hôtes runners restent pilotés depuis le poste. Aucune clé SSH de production,
aucun kubeconfig ni identité OpenBao de production n’entre dans ces jobs.

[Protection, limites et politiques de panne](docs/security-runtime.FR.md).
