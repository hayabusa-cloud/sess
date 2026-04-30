[![Go Reference](https://pkg.go.dev/badge/code.hybscloud.com/sess.svg)](https://pkg.go.dev/code.hybscloud.com/sess)
[![Go Report Card](https://goreportcard.com/badge/github.com/hayabusa-cloud/sess)](https://goreportcard.com/report/github.com/hayabusa-cloud/sess)
[![Coverage Status](https://codecov.io/gh/hayabusa-cloud/sess/graph/badge.svg)](https://codecov.io/gh/hayabusa-cloud/sess)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

[English](README.md) | [简体中文](README.zh-CN.md) | [Español](README.es.md) | [日本語](README.ja.md) | **Français**

# sess

Protocoles de communication à types de session via effets algébriques sur [kont](https://code.hybscloud.com/kont).

## Présentation

Les types de session attribuent un type à chaque étape d'un protocole de communication. Chaque opération — envoyer,
recevoir, sélectionner, offrir, fermer — est individuellement bien typée grâce aux génériques de Go, et la composition
de protocoles au sein d'un même endpoint est sûre en types. La dualité (correspondance des opérations entre endpoints)
est à la charge du programmeur : c'est lui qui écrit les protocoles duaux, et les incohérences se manifestent à
l'exécution sous forme d'échecs d'assertion de type ou de blocages.

`sess` encode les types de session comme des effets algébriques évalués par le système
d'effets [kont](https://code.hybscloud.com/kont). Chaque étape du protocole — envoyer, recevoir, sélectionner, offrir,
fermer — est un effet qui suspend le calcul jusqu'à ce que le transport ait terminé l'opération. Le transport renvoie
`iox.ErrWouldBlock` aux frontières de calcul, ce qui permet aux boucles d'événements proactor (par ex. `io_uring`) de
multiplexer l'exécution sans bloquer les threads.

Deux familles d'API équivalentes sont disponibles : Cont (à base de fermetures, simple à composer) et Expr (à base de
cadres, avec zéro allocation amortie sur les chemins critiques).

## Limite de composition

`sess` possède la signature d'effets de session et le transport d'endpoints qui l'interprète. Il utilise
`iox.ErrWouldBlock` comme frontière non bloquante pour les files bornées, mais ne possède pas l'algèbre complète des
résultats de `iox` ; `takt` possède la planification de style proactor et la corrélation des complétions ; `cove`
possède la preuve contextuelle pour la composition autour des suspensions.

## Installation

```bash
go get code.hybscloud.com/sess
```

Nécessite Go 1.26+.

## Opérations de session

Chaque opération admet une duale. Quand un endpoint exécute une opération, l'autre doit exécuter sa duale.

| Opération                                   | Duale                             | Suspend ?           |
|---------------------------------------------|-----------------------------------|---------------------|
| `Send[T]` — envoyer une valeur              | `Recv[T]` — recevoir une valeur   | `iox.ErrWouldBlock` |
| `SelectL` / `SelectR` — choisir une branche | `Offer` — suivre le choix du pair | `iox.ErrWouldBlock` |
| `Close` — terminer la session               | `Close`                           | Jamais              |

## Utilisation

Utilisez `Run` pour prototyper et valider des protocoles. Utilisez `Exec` avec des endpoints gérés en externe. Utilisez
l'API Expr (`RunExpr`/`ExecExpr`) lorsque vous avez besoin d'un contrôle pas à pas ou que vous voulez minimiser le
surcoût d'allocation sur les chemins critiques.

### Envoi et réception

Un côté envoie une valeur ; le côté dual la reçoit.

```go
client := sess.SendThen(42, sess.CloseDone("ok"))
server := sess.RecvBind(func(n int) kont.Eff[string] {
    return sess.CloseDone(fmt.Sprintf("got %d", n))
})
a, b := sess.Run(client, server) // "ok", "got 42"
```

Équivalent Expr : `ExprSendThen`, `ExprRecvBind`, `ExprCloseDone`, `RunExpr`.

### Branchement

Un côté sélectionne une branche ; le côté dual offre les deux branches et suit le choix effectué.

```go
client := sess.SelectLThen(sess.SendThen(1, sess.CloseDone("left")))
server := sess.OfferBranch(
    func() kont.Eff[string] {
        return sess.RecvBind(func(n int) kont.Eff[string] {
            return sess.CloseDone(fmt.Sprintf("left %d", n))
        })
    },
    func() kont.Eff[string] { return sess.CloseDone("right") },
)
a, b := sess.Run(client, server)
```

### Protocoles récursifs

Les protocoles répétitifs utilisent `Loop` avec `Either` : `Left` poursuit la boucle, `Right` termine.

```go
counter := sess.Loop(0, func(i int) kont.Eff[kont.Either[int, string]] {
    if i >= 3 {
        return sess.CloseDone(kont.Right[int, string]("done"))
    }
    return sess.SendThen(i, kont.Pure(kont.Left[int, string](i+1)))
})
```

### Délégation

Transférez un endpoint à un tiers en l'envoyant ; acceptez la délégation en le recevant.

```go
delegator := sess.SendThen(endpoint, sess.CloseDone("delegated"))
acceptor := sess.RecvBind(func(ep *sess.Endpoint) kont.Eff[string] {
    return sess.CloseDone("accepted")
})
```

### Pas à pas

Pour les boucles d'événements proactor (par ex. `io_uring`), `Step` et `Advance` évaluent un effet à la fois.
Contrairement à `Run` et `Exec` — qui attendent les progrès de manière synchrone — l'API de stepping renvoie
`iox.ErrWouldBlock` à l'appelant, permettant à la boucle d'événements de replanifier.

```go
ep, _ := sess.New()
protocol := sess.ExprSendThen(42, sess.ExprCloseDone[struct{}](struct{}{}))
_, susp := sess.Step[struct{}](protocol)
// Dans une boucle d'événements proactor (par ex. io_uring), céder à la frontière :
_, nextSusp, err := sess.Advance(ep, susp)
if err != nil {
    return susp // céder à la boucle d'événements ; replanifier quand prêt
}
susp = nextSusp
```

### Gestion des erreurs

Composez des protocoles de session avec des effets d'erreur. `Throw` interrompt immédiatement l'exécution appariée. La
valeur `thrown` renvoyée est la cause globale d'un `Throw` non capturé ; consultez-la avant d'interpréter un `Either`
côté pair.

```go
client := kont.ExprThrowError[string, string]("boom")
server := sess.ExprRecvBind(func(v string) kont.Expr[string] {
    return sess.ExprCloseDone("recv: " + v)
})

clientResult, serverResult, thrown := sess.RunErrorExpr[string](client, server)
if thrown != nil {
    // Abandon global de la session.
    fmt.Println("session aborted:", *thrown)
    // Le Either du pair peut encore être localement non résolu.
    _ = clientResult
    _ = serverResult
    return
}

// Aucun Throw global non capturé : les deux Either sont des résultats locaux finaux.
fmt.Println(clientResult, serverResult)
```

En résumé :

- `thrown == nil` : les deux valeurs `Either` sont des résultats locaux finaux.
- `thrown != nil` : l'exécution appariée a été interrompue globalement ; `*thrown` est le `Throw` non capturé et le
  `Either` du pair peut encore être non résolu.

## Modèle d'exécution

| Fonction            | Description                                                                      |
|---------------------|----------------------------------------------------------------------------------|
| `Run` / `RunExpr`   | Exécute les deux côtés sur une goroutine — crée une paire d'endpoints en interne |
| `Exec` / `ExecExpr` | Exécute un côté sur un endpoint pré-créé                                         |
| `Step` + `Advance`  | Évalue un effet à la fois, pour boucles d'événements externes                    |

**Cont vs Expr** : Cont est à base de fermetures et facile à composer. Expr est à base de cadres avec zéro allocation
amortie, adapté aux chemins critiques.

## Contrat

`sess` expose une API de transport destinée à des appelants de confiance. Chaque endpoint est conçu pour être utilisé
par une seule goroutine à la fois, et le chemin critique omet délibérément les contrôles d'usage concurrent ainsi que
les vérifications après `Close`.

Si le type de la charge utile est une interface, la valeur doit tout de même porter un type dynamique concret. Les
valeurs d'interface nil comme `any(nil)` ou `error(nil)` sont hors contrat ; si nil a une signification sémantique,
utilisez une valeur nil d'un type concret ou un wrapper explicite.

## API

| Catégorie              | Cont                                                                             | Expr                                                                                                     |
|------------------------|----------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------|
| Constructeurs          | `SendThen`, `RecvBind`, `CloseDone`, `SelectLThen`, `SelectRThen`, `OfferBranch` | `ExprSendThen`, `ExprRecvBind`, `ExprCloseDone`, `ExprSelectLThen`, `ExprSelectRThen`, `ExprOfferBranch` |
| Récursion              | `Loop`                                                                           | `ExprLoop`                                                                                               |
| Exécution              | `Exec`, `Run`                                                                    | `ExecExpr`, `RunExpr`                                                                                    |
| Exécution avec erreurs | `ExecError`, `RunError`                                                          | `ExecErrorExpr`, `RunErrorExpr`                                                                          |
| Pas à pas              |                                                                                  | `Step`, `Advance`, `StepError`, `AdvanceError`                                                           |
| Pont                   | `Reify` (Cont→Expr), `Reflect` (Expr→Cont)                                       |                                                                                                          |
| Transport              | `New` → `(*Endpoint, *Endpoint)`                                                 |                                                                                                          |

## Schémas pratiques

Une exécution appariée avec gestion d'erreurs définit les protocoles duaux et laisse `RunErrorExpr` créer la paire
d'endpoints en interne :

```go
// 1. Définir le protocole de chaque côté à l'aide des opérations duales.
clientProg := sess.ExprSendThen(42, sess.ExprRecvBind(
    func(reply string) kont.Expr[string] {
        return sess.ExprCloseDone(reply)
    },
))
serverProg := sess.ExprRecvBind(func(n int) kont.Expr[string] {
    return sess.ExprSendThen(
        fmt.Sprintf("got %d", n),
        sess.ExprCloseDone[string]("ok"),
    )
})

// 2. Faire avancer les deux côtés en cadence avec gestion d'erreurs.
type Err struct{ Reason string }
left, right, thrown := sess.RunErrorExpr[Err](clientProg, serverProg)
if thrown != nil {
    // La session a été interrompue ; left/right peuvent ne porter que des résultats partiels.
    _ = thrown
}
_ = left; _ = right
```

Pour l'intégration proactor, créez les endpoints avec `sess.New()` et pilotez `StepError` / `AdvanceError` directement :
la suspension cède dès que le transport sous-jacent renvoie `iox.ErrWouldBlock`, et la boucle la reprend lorsque
l'endpoint correspondant a terminé.

## Références

- Kohei Honda. 1993. Types for Dyadic Interaction. In *Proc. 4th International Conference on Concurrency Theory (
  CONCUR '93)*. LNCS 715, 509–523. https://doi.org/10.1007/3-540-57208-2_35
- Kohei Honda, Vasco T. Vasconcelos, and Makoto Kubo. 1998. Language Primitives and Type Discipline for Structured
  Communication-Based Programming. In *Proc. 7th European Symposium on Programming (ESOP '98)*. LNCS 1381,
  122–138. https://doi.org/10.1007/BFb0053567
- Philip Wadler. 2014. Propositions as Sessions. *Journal of Functional Programming* 24, 2-3 (2014),
  384–418. https://doi.org/10.1017/S095679681400001X
- Dominic A. Orchard and Nobuko Yoshida. 2016. Effects as Sessions, Sessions as Effects. In *Proc. 43rd Annual ACM
  SIGPLAN-SIGACT Symposium on Principles of Programming Languages (POPL '16)*.
  568–581. https://doi.org/10.1145/2837614.2837634
- Sam Lindley and J. Garrett Morris. 2022. Lightweight Functional Session Types. In *Behavioural Types: From Theory to
  Tools*. 265–286. https://doi.org/10.1201/9781003337331-12
- Simon Fowler, Sam Lindley, J. Garrett Morris, and Sára Decova. 2019. Exceptional Asynchronous Session Types: Session
  Types without Tiers. *Proc. ACM Program. Lang.* 3, POPL (Jan. 2019), 1–29. https://doi.org/10.1145/3290341

## Dépendances

- [code.hybscloud.com/kont](https://code.hybscloud.com/kont) — Continuations délimitées et effets algébriques
- [code.hybscloud.com/iox](https://code.hybscloud.com/iox) — Sémantique non bloquante (`ErrWouldBlock`, `Backoff`)
- [code.hybscloud.com/lfq](https://code.hybscloud.com/lfq) — Files FIFO sans verrou

## Licence

MIT — voir [LICENSE](LICENSE).

©2026 [Hayabusa Cloud Co., Ltd.](https://code.hybscloud.com)
