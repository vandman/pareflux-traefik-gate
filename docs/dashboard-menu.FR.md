<!-- FR: Décrit la navigation privée signée et sa configuration. EN: Describes signed private navigation and its setup. Called by: operators; Calls: Gate and Control integration. -->
# Navigation dans le dashboard natif

[Installation](../LISEZMOI.md) · [English](dashboard-menu.EN.md)

Cette intégration facultative ajoute le lien PareFlux au dashboard Traefik et
un retour vers Traefik dans Control. Elle ne déplace pas les applications :
le service du dashboard reste `api@internal`. Gate protège sans console.
L’authentification peut utiliser un SSO ou un autre mécanisme adapté ; les
échanges privés ne transmettent jamais les sessions utilisateur.

## Configuration des deux côtés

Monter la même clef aléatoire d’au moins 32 octets, en lecture seule, dans
Traefik et Control. La générer hors des fichiers suivis par Git, par exemple
avec `openssl rand -hex 32`. Ne pas utiliser la clef LAPI pour cet échange.

Dans Control, adapter ces variables à ses véritables routes :

```yaml
PAREFLUX_HOST: security.example.org
PAREFLUX_BASE_PATH: /console
PAREFLUX_INGRESS_INTEGRATION_URL: http://traefik-private:9000
PAREFLUX_INGRESS_DISCOVERY_KEY_FILE: /run/secrets/pareflux-discovery.key
```

Les chemins de fichiers sont internes à chaque conteneur. `PAREFLUX_HOST` n’a
ni schéma ni chemin ; l’URL d’intégration désigne l’origine privée Traefik,
sans ajouter `/.well-known/pareflux` : Control l’ajoute.

Dans la configuration **statique** Traefik, déclarer un entrypoint privé :

```yaml
entryPoints:
  pareflux-private:
    address: ":9000"
```

Ne pas publier ce port sur Internet. Le réserver à Control par réseau privé,
pare-feu ou NetworkPolicy. Le secret signé ne remplace pas cette restriction.

Dans la configuration **dynamique**, adapter cet exemple au dashboard existant.
`dashboard-auth` représente son middleware d’authentification existant : il faut
le définir ou garder la chaîne équivalente déjà en place. L’API du dashboard
doit être activée dans la configuration statique. Les ancres YAML copient les
options du même moteur dans les deux instances de navigation.

```yaml
http:
  middlewares:
    pareflux-gate:
      plugin:
        parefluxgate: &protection
          enabled: true
          crowdsecMode: stream
          crowdsecLapiHost: crowdsec:8080
          crowdsecLapiKeyFile: /run/secrets/crowdsec-bouncer.key
          crowdsecAppsecEnabled: false
    pareflux-navigation:
      plugin:
        parefluxgate: &navigation
          <<: *protection
          enabled: false
          dashboardIntegration: true
          dashboardHost: traefik.example.org
          dashboardHealthURL: http://pareflux-control:8080/console/readyz
          dashboardDiscoveryKeyFile: /run/secrets/pareflux-discovery.key
          dashboardRegistrationAllowed: false
    pareflux-registration:
      plugin:
        parefluxgate:
          <<: *navigation
          dashboardRegistrationAllowed: true
  routers:
    dashboard:
      rule: Host(`traefik.example.org`) && (PathPrefix(`/dashboard`) || PathPrefix(`/api`))
      entryPoints: [websecure]
      tls: {}
      service: api@internal
      middlewares: [dashboard-auth, pareflux-gate, pareflux-navigation]
    pareflux-private:
      rule: Path(`/.well-known/pareflux`)
      entryPoints: [pareflux-private]
      service: api@internal
      middlewares: [pareflux-registration]
```

Les instances de navigation ont `enabled: false` : elles ne contactent pas
CrowdSec et ne remplacent pas le middleware de protection. Garder exactement les
options du moteur de protection réellement appliqué, y compris après modification
ou rotation de clef, pour que les preuves de protection correspondent.
Seule l’instance privée autorise les annonces signées. La publique garde
`dashboardRegistrationAllowed: false`. Sans clef partagée valide,
`dashboardIntegration` ne peut pas être construit.

Les domaines ci-dessus sont des exemples ; adapter toutes les adresses à son installation.

## Affichage et état

Le HTML 200 non compressé de `/dashboard/` reçoit le lien dans « PLUGINS » et
une tuile « PareFlux / CrowdSec » dans « Features ». Les actifs, erreurs,
redirections et API ne sont pas réécrits. Le tampon HTML est limité à 2 Mio ;
les réponses plus grandes ou diffusées passent sans transformation. Un lien
flottant sert de repli si la structure native n’est pas reconnue.
`/dashboard` redirige vers `/dashboard/` en gardant la requête, après les contrôles
de la route.

« Connecté » signifie que Control est disponible. « Protégé » exige le passage
réel par Gate, un échange CrowdSec récent et une inspection AppSec réussie si
activée. Ces états restent distincts, avec couleur, texte et infobulle. Le
statut est actualisé toutes les 15 secondes sur une route du dashboard protégée
par la même chaîne. Aucun en-tête fourni par le navigateur ne peut le confirmer.

La sonde privée attend au plus 750 ms ; son résultat est conservé 15 secondes.
Seul HTTP 200 avec JSON `status: ready` est accepté, pas une page de connexion.
Un contrôle lent ne garde pas le verrou partagé pendant l’appel réseau.
Tant que l’annonce reste valide, une panne conserve le lien avec avertissement
et infobulle dans les 24 langues officielles de l’UE ou en coréen. **Après
90 secondes sans nouvelle annonce, le lien est retiré**, même si une ancienne
sonde a réussi. Le dashboard reste indépendant de Control.

## Protocole de découverte mutuelle

Control envoie toutes les 30 secondes un POST privé à `/.well-known/pareflux`
avec `control_url`, `health_path` et un `nonce` aléatoire. Requête et réponse
portent `X-Pareflux-Time` et `X-Pareflux-Signature` : une signature HMAC lie le
corps, l’horodatage et le sens de l’échange. Les signatures invalides, annonces
rejouées ou horodatages expirés sont refusés.

La réponse inclut `protocol: pareflux.ingress.v1`, `name`, `dashboard_url`,
`control_url`, `nonce`, `expires_in: 90`, `control_reachable`, `gate_version` et
`range_decisions`. Ce dernier ne confirme que le moteur correspondant, sain et
récent. Control affiche le retour vers l’ingress seulement après échange valide
et le retire à expiration.

La console peut annoncer une nouvelle URL publique. Le chemin de santé reste
résolu sur l’origine privée configurée : une annonce ne peut pas choisir un
autre serveur de sonde. Sans annonce valide, la découverte n’affiche aucun lien.
Pour un simple lien statique sans découverte, utiliser `dashboardLinkURL` et
`dashboardHost` avec `enabled: false`, en laissant `dashboardIntegration: false` ;
ce mode n’offre pas le même échange d’état. Les autres ingress peuvent implémenter
le protocole ; leur plugin n’est pas fourni ici.
