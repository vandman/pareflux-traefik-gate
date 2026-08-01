<!--
###############################################################################
# Politique de securite de PareFlux Gate
# FR: Definit le canal prive et les informations attendues pour signaler une
#     vulnerabilite sans exposer les utilisateurs du plugin.
# EN: Defines the private channel and expected information for reporting a
#     vulnerability without exposing plugin users.
# Called by: Chercheurs, utilisateurs et interface de securite GitHub
# Calls: Signalement prive GitHub et mainteneurs PareFlux
###############################################################################
-->

# Politique de securite

## Versions prises en charge

Les correctifs de securite ciblent la derniere version publiee de PareFlux
Gate. Lorsque cela peut etre fait sans risque, confirmez le probleme avec cette
version avant de le signaler.

## Signaler une vulnerabilite

N'ouvrez pas d'issue publique pour une vulnerabilite, un exploit, un
contournement d'authentification ou d'autorisation, une exposition de secret,
une resolution non sure de l'identite cliente ou toute autre faiblesse de
securite sensible.

Utilisez le [signalement prive de vulnerabilite GitHub](https://github.com/vandman/pareflux-traefik-gate/security/advisories/new).
Indiquez :

- la version affectee et la version de Traefik ;
- l'impact de securite et le modele de menace realiste ;
- une reproduction minimale ou une preuve de concept ;
- la configuration et les journaux pertinents expurges ;
- les attenuations ou contournements connus ;
- l'attribution souhaitee lors de la divulgation, le cas echeant.

N'envoyez jamais d'identifiants actifs, de jetons, de cookies, de cles privees,
de donnees personnelles ou d'informations d'identification inutiles. Les
mainteneurs coordonneront la validation, la correction et la divulgation
responsable dans l'avis prive.

Pour une proposition de durcissement non sensible, utilisez plutot une demande
d'evolution publique.

La version anglaise est disponible dans [SECURITY.md](SECURITY.md).
