# Navigation dans le dashboard natif

Utiliser une instance distincte du plugin PareFlux après les middlewares de
protection et de SSO existants sur le routeur du dashboard Traefik. Garder
l'instance de protection activée. L'instance de navigation utilise `enabled: false`
et `dashboardIntegration: true` ; elle ne contacte pas CrowdSec et ne remplace
pas la protection. Le service reste `api@internal`. PareFlux Control ne sert
aucun HTML du dashboard.

Configurer `dashboardHost` avec le domaine exact du dashboard, `dashboardLinkURL`
avec l'URL publique de la console et `dashboardHealthURL` avec sa disponibilité
privée. Ce sont des paramètres du déploiement ; aucun domaine de site n'est
inscrit dans le code. L'outil de déploiement les déduit des règles de routage,
de l'environnement Control et du Service existants. Une autre installation
peut publier sa console sur `https://security.example/console/`.

Seul le HTML 200 non compressé de l'entrée du dashboard reçoit le lien. Les
actifs, API, erreurs, blocages et redirections restent inchangés. Le tampon est
limité à 2 Mio ; les réponses volumineuses ou diffusées passent sans transformation.
`/dashboard` redirige localement vers `/dashboard/` en conservant la requête,
après autorisation SSO.

Le contrôle privé de disponibilité expire après 750 ms et est mis en cache
15 secondes. Seul un HTTP 200 avec le JSON `status: ready` est considéré prêt ;
une page SSO ne suffit pas. Un service indisponible garde le dernier lien connu,
cliquable avec un avertissement et une infobulle dans les 24 langues officielles
de l'UE ou en coréen selon la langue du navigateur. Le dashboard natif reste
indépendant de Control.

La rubrique native « PLUGINS » contient le lien PareFlux. La tuile « PareFlux /
CrowdSec » dans « Features » distingue « Connecté » (console disponible) et
« Protégé » (passage effectif par Gate, échange CrowdSec récent et inspection
AppSec réussie si activée). Les états sont verts quand confirmés, rouges sinon,
avec un libellé et une infobulle. Le statut est actualisé toutes les 15 secondes
sur une route du dashboard protégée par les middlewares existants. Aucun en-tête
fourni par le navigateur ne peut confirmer la protection. Si la structure native
n'est pas reconnue, un lien flottant reste disponible.

## Protocole de découverte mutuelle

Exposer `POST /.well-known/pareflux` uniquement sur un entrypoint interne réservé
à Control par politique réseau. Ne jamais le publier par le routeur public du
dashboard. Control annonce en JSON `control_url`, `health_path` et un `nonce`
aléatoire toutes les 30 secondes. Le plugin répond avec le protocole
`pareflux.ingress.v1`, le produit `name`, `dashboard_url`, `control_url` accepté,
le `nonce` reçu, `expires_in: 90` et `control_reachable`. Aucun cookie utilisateur
ni identifiant SSO n'y participe. Control affiche le retour vers l'ingress
uniquement après un échange valide et le masque en cas d'absence ou d'expiration.

La console peut annoncer une nouvelle URL publique sans reconstruire le plugin.
Son chemin de disponibilité peut également changer ; il reste résolu uniquement
sur l'origine privée configurée, empêchant l'annonce de choisir un autre serveur.
La dernière URL annoncée reste proposée avec avertissement pendant une panne.
Sans point d'intégration, une instance est autonome et n'affiche aucun retour.
Les plugins d'autres ingress peuvent implémenter ce protocole ; ils ne sont pas fournis ici.
