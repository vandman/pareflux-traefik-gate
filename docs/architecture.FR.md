<!--
FR: Décrit les composants et décisions du middleware sans alourdir le guide d’installation.
EN: Describes middleware components and decisions separately from the installation guide.
Called by: README and LISEZMOI; Calls: Mermaid diagrams.
-->
# Architecture de PareFlux Gate

Ce document décrit le traitement des requêtes, les échanges réseau, le cache et les politiques de panne. Les schémas Mermaid sont rendus sur GitHub.

[Retour à l’installation](../LISEZMOI.md) · [English](architecture.EN.md)

## Architecture

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

## Flux réseau

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

## Flux applicatif

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

## Flux de décision de ban

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
    ferme -->|Non, configuration explicite| appsec
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
