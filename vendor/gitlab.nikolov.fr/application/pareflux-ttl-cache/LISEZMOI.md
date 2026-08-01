# Cache TTL PareFlux

PareFlux TTL Cache est un petit cache mémoire Apache-2.0 destiné aux décisions
de sécurité et aux middlewares Traefik.

## Contrat

- Les clés sont des chaînes et les valeurs sont des valeurs Go quelconques.
- Un TTL positif est exprimé en secondes.
- Un TTL nul ne modifie pas le cache.
- Un TTL négatif n'expire jamais.
- Les entrées expirées sont supprimées à la lecture ou par nettoyage borné.
- `New` borne le cache à 65 536 entrées et évince en temps constant l'entrée
  insérée ou renouvelée la plus ancienne ; `NewWithCapacity` permet de choisir
  explicitement une autre limite.
- Le cache ne lance aucune goroutine et ne réalise aucune persistance.

## Utilisation

```go
cache := ttlcache.New()
cache.Set("198.51.100.10", "ban", 60)
value, found := cache.Get("198.51.100.10")
```

## Vérification

```bash
go test -race ./...
go test -bench=. -benchmem -count=10 -cpu=1,4,16 ./...
```

Les benchmarks natifs ne suffisent pas. Toute version consommée par PareFlux
Gate est également mesurée dans le runtime Traefik/Yaegi correspondant.

## Politique de licence

Le module est sous Apache-2.0 et ne possède volontairement aucune dépendance
d'exécution. Les dépendances GPL-3.0, AGPL ou de licence inconnue sont refusées.
