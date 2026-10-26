<!--
FR: Décrit la préparation automatique des liens publics avant publication du plugin.
EN: Describes automatic public-link preparation before publishing the plugin.
Called by: maintainers; Calls: publish-github.sh and catalog-links.
-->
# Publier la documentation du catalogue

[English](publication.EN.md) · [Retour au guide](../LISEZMOI.md)

Le catalogue Traefik affiche le README de la version publique sur son propre
site. Un lien relatif comme `LISEZMOI.md` pointerait donc vers le catalogue,
et non vers le dépôt. La publication prépare automatiquement les liens du
seul README affiché par Traefik, sans remplacement manuel à chaque version.
Les autres documents restent sur GitHub et conservent leurs liens relatifs.
Cliquer sur un lien vers un autre document quitte le catalogue.

## Publier une version

Commiter les sources validées et choisir un numéro stable encore inutilisé.
Le tag GitLab principal est `publish/vX.Y.Z` ; le tag GitHub public est `vX.Y.Z`.
Les tags existants sont immuables. GitLab est la source de référence et sa CI
est le seul chemin de publication vers GitHub. Après les vérifications locales :

```bash
PUBLISH_DRY_RUN=1 PAREFLUX_GITHUB_REPOSITORY=https://github.com/vandman/pareflux-traefik-gate.git ./scripts/publish-github.sh vX.Y.Z
git push origin HEAD:main
git tag -a publish/vX.Y.Z -m "PareFlux Gate vX.Y.Z"
git push origin refs/tags/publish/vX.Y.Z
```

La pipeline du tag protégé exécute les tests Go et l'intégration Traefik/Yaegi,
puis publie l'export et le tag `vX.Y.Z` sur GitHub de manière atomique. La variable
GitLab `PAREFLUX_GITHUB_DEPLOY_KEY` doit être de type fichier et protégée ; sa clé
publique doit avoir le droit d'écriture sur le seul dépôt GitHub du plugin.
Une clé absente fait échouer le job de publication au lieu de l'ignorer.
Consulter la pipeline du tag pour confirmer la publication. En cas d'échec,
corriger la cause et relancer le job approprié ; ne pas publier depuis le poste.

L'export isolé utilise `.local/publication-work`, ignoré par Git, et est supprimé
à la fin. Git, Bash et Go ou Docker sont nécessaires. La simulation locale
ne publie rien. Aucun tag existant ne doit être déplacé ou republié.

## Préparation des liens

`scripts/catalog-links` est un utilitaire Go sans dépendance externe, appelé
automatiquement avant les contrôles existants de compilation et de licences :

- Dans le README uniquement, les documents pointent vers les pages GitHub `blob/<version>/...`. Les liens de navigation utilisent `www.github.com`, qui redirige vers les pages GitHub habituelles et évite que le catalogue transforme les liens `github.com` en téléchargements raw.
- Les dossiers pointent vers les pages GitHub `tree/<version>/...`.
- Les racines raw utilisées comme liens de navigation sont converties en pages du dépôt ; les images raw doivent cibler un fichier.
- Les images utilisent des URL complètes `raw.githubusercontent.com/<auteur>/<dépôt>/<version>/<fichier>`.
- Les chemins relatifs, liens de langue, références, liens HTML et sommaires sont résolus depuis leur document source.
- Les liens du dépôt vers `main` ou `master` sont figés sur la version publiée.
- Une cible locale absente, un titre Markdown introuvable, un chemin sortant du dépôt ou une URL raw incomplète bloque la publication.
- Les exemples, le code en ligne et les liens vers les sections du README sont conservés.
- Aucun autre fichier documentaire n’est réécrit.
- Les sites externes et le lien du dépôt GitLab de développement sont conservés ; leur disponibilité n’est pas vérifiée comme celle d’un fichier local.

La racine d’un dépôt sur le domaine raw n’est pas une URL de téléchargement
valide. Utiliser la page GitHub pour parcourir les fichiers, ou préciser la
version et le chemin complet pour du contenu brut. L’outil vérifie toutes les
cibles locales du README avant d’écrire sa version convertie, puis effectue un second
passage de contrôle sans écriture.

Le catalogue indexe GitHub de manière asynchrone. Publier un tag ne rafraîchit
pas immédiatement sa fiche ; vérifier la version affichée avant de comparer
ses liens avec ceux de la dernière documentation publiée.
