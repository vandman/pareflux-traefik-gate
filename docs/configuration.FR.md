<!--
FR: Référence des options vérifiées contre le constructeur et leur utilisation.
EN: Reference of options verified against constructor defaults and runtime usage.
Called by: README and LISEZMOI; Calls: configuration.New and runtime settings.
-->
# Référence de configuration

[Installation](../LISEZMOI.md) · [English](configuration.EN.md)

Voici les noms JSON/YAML des options et leurs valeurs par défaut.
Les durées sont en secondes sauf indication
contraire ; les listes sont des séquences YAML. Une option `File` prime sur sa
valeur directe et désigne un fichier lisible **dans Traefik**. Une valeur vide
ne signifie pas que le service fonctionne sans identifiants. AppSec hérite du
schéma et du TLS LAPI sauf remplacement ; dans les modes LAPI, il hérite aussi
de sa clef. En mode `alone`, fournir une clef de bouncer AppSec distincte si
l’inspection HTTP est activée : les identifiants centraux ne sont pas une clef AppSec.
Copier seulement les réglages nécessaires ; le guide choisit explicitement `stream`.


## Général

| Option | Type | Défaut | Rôle |
| --- | --- | --- | --- |
| `enabled` | `bool` | `false` | Activer la protection ; désactivé, le middleware laisse passer les requêtes. |
| `logLevel` | `string` | `"INFO"` | Niveau de journalisation : DEBUG, INFO, WARN ou ERROR. |
| `logFormat` | `string` | `"common"` | Format des logs : common (texte) ou json. |
| `logFilePath` | `string` | `""` | Fichier de logs ; vide utilise la sortie standard. |
| `crowdsecMode` | `string` | `"live"` | Mode de décision : stream, live, none, appsec ou alone. |
| `updateIntervalSeconds` | `int64` | `60` | Secondes entre deux flux ; le mode alone impose 7200 secondes. |
| `metricsUpdateIntervalSeconds` | `int64` | `600` | Période d’envoi des métriques de remédiation à CrowdSec ; zéro désactive la tâche. |
| `updateMaxFailure` | `int64` | `3` | Échecs consécutifs du flux tolérés avant l’état en échec ; 0 dès le premier, -1 désactive cette transition. |
| `defaultDecisionSeconds` | `int64` | `60` | Durée maximale du cache live en secondes, limitée par la durée de décision. |
| `remediationStatusCode` | `int` | `403` | Code HTTP des blocages de sécurité ; les erreurs techniques utilisent séparément 408, 413 ou 503. |
| `httpTimeoutSeconds` | `int64` | `10` | Délai maximal HTTP LAPI/CAPI en secondes. |
| `streamDecisionCapacity` | `int64` | `262144` | Plafond de décisions actives (maximum 1000000) ; aucune éviction de ban actif, refus en cas de saturation. |
| `failureGraceSeconds` | `int64` | `60` | Grâce depuis la dernière synchronisation ou inspection AppSec réussie, sous réserve des seuils d’échec. |

## LAPI et TLS

| Option | Type | Défaut | Rôle |
| --- | --- | --- | --- |
| `crowdsecLapiScheme` | `string` | `"http"` | Transport LAPI : http ou https. |
| `crowdsecLapiHost` | `string` | `"crowdsec:8080"` | Nom et port LAPI, joignables depuis Traefik. |
| `crowdsecLapiPath` | `string` | `"/"` | Chemin de base précédant les endpoints v1 de LAPI. |
| `crowdsecLapiKey` | `string` | `""` | Clef API du bouncer ; préférer sa variante File en production. |
| `crowdsecLapiKeyFile` | `string` | `""` | Lire crowdsecLapiKey dans ce fichier à l’intérieur de Traefik ; prioritaire sur la valeur directe. |
| `crowdsecLapiTlsInsecureVerify` | `bool` | `false` | Désactiver la vérification du certificat serveur ; garder false en production. |
| `crowdsecLapiTlsCertificateAuthority` | `string` | `""` | Autorité de certification PEM servant à vérifier le serveur. |
| `crowdsecLapiTlsCertificateAuthorityFile` | `string` | `""` | Lire crowdsecLapiTlsCertificateAuthority dans ce fichier à l’intérieur de Traefik ; prioritaire sur la valeur directe. |
| `crowdsecLapiTlsCertificateBouncer` | `string` | `""` | Certificat client PEM pour l’authentification TLS mutuelle. |
| `crowdsecLapiTlsCertificateBouncerFile` | `string` | `""` | Lire crowdsecLapiTlsCertificateBouncer dans ce fichier à l’intérieur de Traefik ; prioritaire sur la valeur directe. |
| `crowdsecLapiTlsCertificateBouncerKey` | `string` | `""` | Clef privée PEM correspondant au certificat client. |
| `crowdsecLapiTlsCertificateBouncerKeyFile` | `string` | `""` | Lire crowdsecLapiTlsCertificateBouncerKey dans ce fichier à l’intérieur de Traefik ; prioritaire sur la valeur directe. |
| `crowdsecStreamUnreachableBlock` | `bool` | `true` | Refuser les clients inconnus après le seuil d’échecs et la grâce ; les blocages connus non expirés restent appliqués. |

## AppSec

| Option | Type | Défaut | Rôle |
| --- | --- | --- | --- |
| `crowdsecAppsecEnabled` | `bool` | `false` | Activer l’inspection HTTP ; nécessaire même en mode appsec. |
| `crowdsecAppsecScheme` | `string` | `""` | Vide hérite du schéma et du TLS LAPI ; sinon utilise les réglages de transport AppSec. |
| `crowdsecAppsecHost` | `string` | `"crowdsec:7422"` | Nom et port AppSec, joignables depuis Traefik. |
| `crowdsecAppsecPath` | `string` | `"/"` | Chemin de l’endpoint d’inspection AppSec. |
| `crowdsecAppsecKey` | `string` | `""` | Clef AppSec ; vide hérite de la clef LAPI dans les modes LAPI. |
| `crowdsecAppsecKeyFile` | `string` | `""` | Lire crowdsecAppsecKey dans ce fichier à l’intérieur de Traefik ; prioritaire sur la valeur directe. |
| `crowdsecAppsecTlsInsecureVerify` | `bool` | `false` | Désactiver la vérification du certificat serveur ; garder false en production. |
| `crowdsecAppsecTlsCertificateAuthority` | `string` | `""` | Autorité de certification PEM servant à vérifier le serveur. |
| `crowdsecAppsecTlsCertificateAuthorityFile` | `string` | `""` | Lire crowdsecAppsecTlsCertificateAuthority dans ce fichier à l’intérieur de Traefik ; prioritaire sur la valeur directe. |
| `crowdsecAppsecTlsCertificateBouncer` | `string` | `""` | Certificat client PEM pour l’authentification TLS mutuelle. |
| `crowdsecAppsecTlsCertificateBouncerFile` | `string` | `""` | Lire crowdsecAppsecTlsCertificateBouncer dans ce fichier à l’intérieur de Traefik ; prioritaire sur la valeur directe. |
| `crowdsecAppsecTlsCertificateBouncerKey` | `string` | `""` | Clef privée PEM correspondant au certificat client. |
| `crowdsecAppsecTlsCertificateBouncerKeyFile` | `string` | `""` | Lire crowdsecAppsecTlsCertificateBouncerKey dans ce fichier à l’intérieur de Traefik ; prioritaire sur la valeur directe. |
| `crowdsecAppsecFailureBlock` | `bool` | `true` | Refuser les HTTP 502/503/504 après grâce ; false les autorise. HTTP 500 reste bloquant. |
| `crowdsecAppsecUnreachableBlock` | `bool` | `true` | Refuser les échecs de connexion après grâce ; false les autorise. Les expirations de connexion établie restent bloquantes. |
| `crowdsecAppsecBodyLimit` | `int64` | `1048576` | Préfixe maximal du corps inspecté en octets ; zéro désactive le corps, pas les contrôles des métadonnées. |
| `crowdsecAppsecBodyLimitAction` | `string` | `"reject"` | Dépassement : refus 413 ou inspection partielle explicitement autorisée pour ce middleware. |
| `crowdsecAppsecBodyExceptions` | `[]object` | `[]` | Jusqu’à 32 exceptions de téléversement : host exact, pathRegexp ancrée et méthodes POST/PUT/PATCH. |
| `crowdsecAppsecBodyExcludedContentTypes` | `[]string` | `[]` | Compatibilité seulement ; ne désactive plus l’inspection. |
| `crowdsecAppsecReadTimeoutSeconds` | `int64` | `60` | Délai fini du corps après inspection ; aligner avec Traefik, 0 laisse ses échéances intactes. |
| `crowdsecAppsecMaxConcurrentPerClient` | `int64` | `4` | Corps préchargés par IPv4 ou IPv6 /64 ; quota IPv6 /48 quatre fois supérieur. Maximum 64 ; 0 utilise 4 ; requêtes sans corps non comptées. |
| `crowdsecAppsecTimeoutSeconds` | `int64` | `3` | Budget total de lecture du corps et d’inspection AppSec ; 3 secondes par défaut, 0 utilise aussi 3. |

## Identité client et logs

| Option | Type | Défaut | Rôle |
| --- | --- | --- | --- |
| `traceHeadersCustomName` | `string` | `""` | En-tête de requête exposé comme TraceID au modèle de ban personnalisé. |
| `remediationHeadersCustomName` | `string` | `""` | Marqueur assaini dans la requête interne et la réponse de refus ; le conserver dans les logs pour filtrer les boucles. |
| `forwardedHeadersCustomName` | `string` | `"X-Forwarded-For"` | En-tête d’IP client utilisé uniquement si le pair immédiat est approuvé. |
| `forwardedHeadersTrustedIps` | `[]string` | `[]` | IP/CIDR des proxies approuvés ; n’exempte pas leurs visiteurs de protection. |
| `clientTrustedIps` | `[]string` | `[]` | IP/CIDR clients résolus exemptés de tous les contrôles Gate, dont AppSec. |

## Redis

| Option | Type | Défaut | Rôle |
| --- | --- | --- | --- |
| `redisCacheEnabled` | `bool` | `false` | Utiliser Redis pour live et captcha ; les décisions stream/alone restent dans leur magasin en mémoire distinct. |
| `redisCacheHost` | `string` | `"redis:6379"` | Nom et port du serveur Redis. |
| `redisCachePassword` | `string` | `""` | Mot de passe Redis ; préférer la variante File. |
| `redisCachePasswordFile` | `string` | `""` | Lire redisCachePassword dans ce fichier à l’intérieur de Traefik ; prioritaire sur la valeur directe. |
| `redisCacheDatabase` | `string` | `""` | Numéro de base Redis sous forme de chaîne ; vide utilise la base par défaut. |
| `redisCacheUnreachableBlock` | `bool` | `true` | Refuser une requête nécessitant Redis s’il est inaccessible ; la recherche IP stream n’utilise pas Redis. |

## Captcha et pages de refus

| Option | Type | Défaut | Rôle |
| --- | --- | --- | --- |
| `banHtmlFilePath` | `string` | `""` | Modèle HTML Go de refus facultatif ; vide retourne seulement le statut. |
| `captchaHtmlFilePath` | `string` | `"/captcha.html"` | Modèle HTML Go du captcha ; à monter dans Traefik. |
| `captchaProvider` | `string` | `""` | Fournisseur captcha : hcaptcha, recaptcha, turnstile ou custom ; vide désactive les challenges. |
| `captchaCustomJsUrl` | `string` | `""` | URL JavaScript du fournisseur captcha personnalisé. |
| `captchaCustomValidateUrl` | `string` | `""` | URL de validation serveur du fournisseur personnalisé. |
| `captchaCustomKey` | `string` | `""` | Classe HTML attendue par le fournisseur personnalisé. |
| `captchaCustomResponse` | `string` | `""` | Champ de réponse du formulaire attendu par le fournisseur personnalisé. |
| `captchaSiteKey` | `string` | `""` | Clef publique du site captcha. |
| `captchaSiteKeyFile` | `string` | `""` | Lire captchaSiteKey dans ce fichier à l’intérieur de Traefik ; prioritaire sur la valeur directe. |
| `captchaSecretKey` | `string` | `""` | Secret de validation captcha. |
| `captchaSecretKeyFile` | `string` | `""` | Lire captchaSecretKey dans ce fichier à l’intérieur de Traefik ; prioritaire sur la valeur directe. |
| `captchaGracePeriodSeconds` | `int64` | `1800` | Secondes avant un nouveau captcha après une validation réussie. |

## API centrale

| Option | Type | Défaut | Rôle |
| --- | --- | --- | --- |
| `crowdsecCapiMachineId` | `string` | `""` | Identifiant de machine enregistrée pour le mode alone. |
| `crowdsecCapiMachineIdFile` | `string` | `""` | Lire crowdsecCapiMachineId dans ce fichier à l’intérieur de Traefik ; prioritaire sur la valeur directe. |
| `crowdsecCapiPassword` | `string` | `""` | Mot de passe de la machine pour le mode alone. |
| `crowdsecCapiPasswordFile` | `string` | `""` | Lire crowdsecCapiPassword dans ce fichier à l’intérieur de Traefik ; prioritaire sur la valeur directe. |
| `crowdsecCapiScenarios` | `[]string` | `[]` | Liste des scénarios transmise lors de l’enregistrement/authentification CAPI. |

## Intégration dashboard

| Option | Type | Défaut | Rôle |
| --- | --- | --- | --- |
| `dashboardHealthURL` | `string` | `""` | URL privée de disponibilité Control ; jamais un identifiant navigateur ni une page SSO publique. |
| `dashboardIntegration` | `bool` | `false` | Activer l’échange privé d’URL sur un middleware de navigation distinct avec enabled=false. |
| `dashboardLinkURL` | `string` | `""` | URL HTTPS publique de Control ou chemin absolu sûr ; lien initial déduit du déploiement. |
| `dashboardHost` | `string` | `""` | Nom d’hôte exact du dashboard natif, sans schéma ni chemin. |
| `dashboardDiscoveryKey` | `string` | `""` | Clé HMAC partagée, au moins 32 octets ; préférer un fichier secret. |
| `dashboardDiscoveryKeyFile` | `string` | `""` | Lit la clé partagée dans un fichier accessible dans Traefik. |
| `dashboardRegistrationAllowed` | `bool` | `false` | Active les annonces signées uniquement sur l’instance privée de navigation. |

[Limites et politiques du moteur](security-runtime.FR.md)
