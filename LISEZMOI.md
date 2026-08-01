# PareFlux Traefik Gate

Documentation : [English](README.md) | **Français**

PareFlux Gate est un middleware HTTP Traefik sous Apache-2.0 pour les décisions
CrowdSec, CrowdSec AppSec et la mise en cache rapide des décisions.

Il dérive de `maxlerebourg/crowdsec-bouncer-traefik-plugin` v1.6.0 sans
importer son historique Git ni sa dépendance de cache GPL-3.0. La provenance
est décrite dans `UPSTREAM.md`.

## Chemin de décision

1. Résoudre l'adresse du pair réseau ou une identité transférée explicitement
   approuvée.
2. Lire le cache TTL éphémère PareFlux.
3. Interroger ou suivre les décisions CrowdSec LAPI.
4. Soumettre les requêtes HTTP éligibles à CrowdSec AppSec.
5. Retourner la remédiation configurée ou appeler le handler suivant.

Les décisions persistantes restent sous l'autorité de CrowdSec LAPI. Le cache
local est vidé au redémarrage de Traefik.

Pour les adresses sans décision en cache, le cache de flux fonctionne en mode
fermé lorsque LAPI est temporairement inaccessible. Définir
`crowdsecStreamUnreachableBlock: false` uniquement si une politique privilégiant
explicitement la disponibilité est requise. Les bans déjà en cache restent
appliqués dans les deux modes.

Gate utilise par défaut l'adresse réseau du pair. Il ne consulte
`X-Forwarded-For` que si le pair immédiat appartient à
`forwardedHeadersTrustedIps` ; les chaînes transmises mal formées sont rejetées.

`remediationHeadersCustomName` est écrit dans la réponse de rejet et dans les
en-têtes de requête préalablement assainis en interne. Conserver cet en-tête
dans le journal d'accès Traefik permet à CrowdSec d'ignorer les rejets produits
par PareFlux au lieu de les réinjecter dans les scénarios de sondage HTTP.

AppSec continue de contrôler les métadonnées des requêtes gRPC et WebSocket,
mais ne précharge pas leurs corps diffusés en continu.

## Intégration avec Traefik

Gate est un plugin local Traefik, pas un proxy inverse ni un sidecar. Traefik
charge le code Go avec Yaegi et appelle le middleware dans son propre pipeline
HTTP. Le plugin partage donc le processus et les objets de requête de Traefik,
mais ouvre ses propres connexions HTTP internes vers CrowdSec LAPI et AppSec.

Le projet installe le code du plugin en lecture seule, monte sa clef LAPI depuis
un fichier secret en lecture seule et référence le middleware depuis les
routeurs ou entrypoints protégés. Une panne de Gate ne doit jamais nécessiter
un accès direct au socket Docker ni au réseau hôte.

### Architecture

```mermaid
flowchart TB
    client["Client HTTP"]

    subgraph processus["Processus Traefik"]
        entree["Entrypoint et routeur"]

        subgraph gate["Middleware PareFlux Gate"]
            identite["Résolution de l'identité source"]
            politique["Politique de décision et de panne"]
            cache["PareFlux TTL Cache"]
            clientappsec["Client AppSec"]
            marqueur["Marqueur de remédiation"]
        end

        service["Handler de service Traefik"]
    end

    lapi["CrowdSec LAPI<br/>flux de décisions"]
    appsec["CrowdSec AppSec<br/>inspection HTTP"]
    backend["Backend protégé"]
    journal["Journal d'accès JSON Traefik"]

    client --> entree --> identite --> politique
    politique <--> cache
    politique <--> lapi
    politique --> clientappsec <--> appsec
    politique --> marqueur
    politique --> service --> backend
    entree --> journal
```

### Flux réseau

```mermaid
flowchart LR
    internet["Réseau client"]

    subgraph conteneurtraefik["Conteneur Traefik"]
        tls["Entrypoints Traefik<br/>:80, :443, :8448 optionnel"]
        gate["Gate s'exécute dans le processus<br/>aucun port d'écoute"]
    end

    subgraph front["Réseau interne front"]
        lapi["CrowdSec LAPI :8080"]
        appsec["CrowdSec AppSec :7422"]
        backend["Port du service applicatif"]
    end

    secret["Fichier de clef LAPI en lecture seule"]
    journal["Fichier access.log persistant"]

    internet --> tls --> gate
    gate -->|"flux de décisions"| lapi
    gate -->|"requête d'inspection"| appsec
    gate -->|"requête autorisée via Traefik"| backend
    secret --> gate
    tls --> journal
```

Le client ne se connecte jamais directement à Gate, LAPI ou AppSec. Le seul
listener public est un entrypoint Traefik. Les plages d'en-têtes transférés de
confiance déterminent quand Gate peut tirer l'identité cliente d'un en-tête de
proxy plutôt que du pair réseau.

### Flux applicatif

```mermaid
sequenceDiagram
    participant C as Client
    participant T as Routeur Traefik
    participant G as PareFlux Gate
    participant K as Cache TTL
    participant L as CrowdSec LAPI
    participant W as CrowdSec AppSec
    participant A as Application

    L-->>G: Mise à jour périodique du flux de décisions
    G->>K: Ajout ou suppression des décisions expirables
    C->>T: Requête HTTP
    T->>G: Appel du middleware
    G->>G: Résolution de l'identité cliente approuvée
    G->>K: Recherche de la décision
    alt Ban ou captcha en cache
        K-->>G: Remédiation
        G-->>C: Statut configuré et marqueur de remédiation
    else Aucune remédiation en cache
        G->>W: Méthode, URI, hôte, en-têtes, IP et corps éligible
        alt AppSec rejette
            W-->>G: Statut de blocage
            G-->>C: Statut configuré et marqueur de remédiation
        else AppSec autorise ou un fail-open configuré s'applique
            W-->>G: Autorisation
            G->>A: Appel du handler Traefik suivant
            A-->>C: Réponse applicative
        end
    end
```

### Flux de décision de ban

```mermaid
flowchart TD
    requete["Traefik appelle Gate"]
    actif{"Gate est-il activé ?"}
    identite["Résoudre le pair réseau ou l'identité transférée approuvée"]
    identiteok{"Identité valide ?"}
    confiance{"IP cliente explicitement approuvée ?"}
    mode{"Le mode de décision utilise-t-il CrowdSec ?"}
    recherche["Chercher l'IP dans le cache TTL ou Redis"]
    trouve{"Valeur en cache ?"}
    remediation["Retourner la remédiation ban ou captcha<br/>et le marqueur interne"]
    flux{"Le flux de décisions est-il sain ?"}
    ferme{"Fail-closed du flux explicitement activé ?"}
    technique["Retourner un rejet technique"]
    appsec{"AppSec est-il activé ?"}
    inspection["Envoyer métadonnées et corps éligible à AppSec"]
    resultat{"Résultat AppSec ?"}
    politiquewaf{"La politique de panne bloque-t-elle cette erreur ?"}
    suivant["Appeler le handler Traefik suivant"]

    requete --> actif
    actif -->|Non| suivant
    actif -->|Oui| identite --> identiteok
    identiteok -->|Non| technique
    identiteok -->|Oui| confiance
    confiance -->|Oui| suivant
    confiance -->|Non| mode
    mode -->|Non ou AppSec seul| appsec
    mode -->|Oui| recherche --> trouve
    trouve -->|Ban ou captcha| remediation
    trouve -->|Autorisation explicite| appsec
    trouve -->|Absent| flux
    flux -->|Sain ou mode sans flux| appsec
    flux -->|Indisponible| ferme
    ferme -->|Oui| technique
    ferme -->|Non, défaut| appsec
    appsec -->|Non| suivant
    appsec -->|Oui| inspection --> resultat
    resultat -->|Autorisation| suivant
    resultat -->|Blocage| remediation
    resultat -->|Indisponible ou erreur interne| politiquewaf
    politiquewaf -->|Bloquer| technique
    politiquewaf -->|Fail-open| suivant
```

Les bans en cache restent appliqués pendant une panne LAPI. Lors d'un défaut de
cache, la politique de flux est fermée par défaut ;
`crowdsecStreamUnreachableBlock: false` active explicitement le mode ouvert. Les
politiques d'indisponibilité et d'erreur interne AppSec se configurent
indépendamment.

## Vérification

```bash
go test -race ./...
go test -bench=. -benchmem ./...
go mod vendor
```

Chaque version doit aussi être chargée comme plugin local dans l'image Traefik
épinglée et réussir les benchmarks de requêtes de bout en bout.

## Dépôts et versions publiques

PareFlux est maintenu par un auteur français qui souhaite disposer d'une
documentation complète et de premier niveau en français. Une version anglaise
complète est maintenue en parallèle, car l'anglais est la langue internationale
de fait pour la collaboration technique et l'écosystème Traefik.

Le [dépôt GitLab](https://github.com/vandman/pareflux-traefik-gate)
est le dépôt de développement principal et contient les commits intermédiaires
et les versions de développement. Le [dépôt GitHub](https://github.com/vandman/pareflux-traefik-gate)
est le canal public de distribution et de contribution, car le catalogue de
plugins Traefik accepte uniquement les dépôts GitHub publics.

Seuls des instantanés stables sont promus vers GitHub. Un tag GitLab protégé
nommé `publish/vX.Y.Z` crée un unique commit public et le tag GitHub immuable
`vX.Y.Z`. Les issues et pull requests publiques sont les bienvenues ; leur
parcours d'intégration est décrit dans
[CONTRIBUTING.FR.md](CONTRIBUTING.FR.md).

## Licence

Apache-2.0. Les dépendances GPL-3.0, AGPL et de licence inconnue sont refusées.
