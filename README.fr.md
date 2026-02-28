[![Go Reference](https://pkg.go.dev/badge/code.hybscloud.com/sess.svg)](https://pkg.go.dev/code.hybscloud.com/sess)
[![Go Report Card](https://goreportcard.com/badge/github.com/hayabusa-cloud/sess)](https://goreportcard.com/report/github.com/hayabusa-cloud/sess)
[![Coverage Status](https://codecov.io/gh/hayabusa-cloud/sess/graph/badge.svg)](https://codecov.io/gh/hayabusa-cloud/sess)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

[English](README.md) | [简体中文](README.zh-CN.md) | [Español](README.es.md) | [日本語](README.ja.md) | **Français**

# sess

Protocoles de communication a types de session via effets algebriques sur [kont](https://code.hybscloud.com/kont).

## Presentation

Les types de session assignent un type a chaque etape d'un protocole de communication. Chaque operation — envoyer, recevoir, selectionner, offrir, fermer — est individuellement bien typee via les generiques de Go, et la composition de protocoles au sein d'un meme endpoint est sure en types. La dualite (correspondance des operations entre endpoints) est une responsabilite du programmeur : le programmeur ecrit des protocoles duaux, et les incoherences se manifestent a l'execution sous forme d'echecs d'assertion de type ou de deadlocks.

`sess` encode les types de session comme des effets algebriques evalues par le systeme d'effets [kont](https://code.hybscloud.com/kont). Chaque etape du protocole — envoyer, recevoir, selectionner, offrir, fermer — est un effet qui suspend le calcul jusqu'a ce que le transport complete l'operation. Le transport retourne `iox.ErrWouldBlock` aux frontieres computationnelles, permettant aux boucles d'evenements proactor (ex., `io_uring`) de multiplexer l'execution sans bloquer les threads.

Deux APIs equivalentes : Cont (base sur les fermetures, composition directe) et Expr (base sur les cadres, zero allocation amortie pour les chemins critiques).

## Installation

```bash
go get code.hybscloud.com/sess
```

Necessite Go 1.26+.

## Operations de Session

Chaque operation a un dual. Quand un endpoint effectue une operation, l'autre doit effectuer son dual.

| Operation | Dual | Suspend ? |
|-----------|------|-----------|
| `Send[T]` — envoyer une valeur | `Recv[T]` — recevoir une valeur | `iox.ErrWouldBlock` |
| `SelectL` / `SelectR` — choisir une branche | `Offer` — suivre le choix du pair | `iox.ErrWouldBlock` |
| `Close` — terminer la session | `Close` | Jamais |

## Utilisation

Utilisez `Run` pour le prototypage et la validation de protocoles. Utilisez `Exec` pour les endpoints gérés en externe. Utilisez l'API Expr (`RunExpr`/`ExecExpr`) pour le contrôle en pas à pas ou pour minimiser la surcharge d'allocation sur les chemins critiques.

### Envoi et Reception

Un cote envoie une valeur ; le cote dual la recoit.

```go
client := sess.SendThen(42, sess.CloseDone("ok"))
server := sess.RecvBind(func(n int) kont.Eff[string] {
    return sess.CloseDone(fmt.Sprintf("got %d", n))
})
a, b := sess.Run(client, server) // "ok", "got 42"
```

Equivalent Expr : `ExprSendThen`, `ExprRecvBind`, `ExprCloseDone`, `RunExpr`.

### Branchement

Un cote selectionne une branche ; le cote dual offre les deux branches et suit la selection.

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

### Protocoles Recursifs

Les protocoles repetitifs utilisent `Loop` avec `Either` : `Left` continue la boucle, `Right` termine.

```go
counter := sess.Loop(0, func(i int) kont.Eff[kont.Either[int, string]] {
    if i >= 3 {
        return sess.CloseDone(kont.Right[int, string]("done"))
    }
    return sess.SendThen(i, kont.Pure(kont.Left[int, string](i+1)))
})
```

### Delegation

Transferez un endpoint a un tiers en l'envoyant ; acceptez la delegation en le recevant.

```go
delegator := sess.SendThen(endpoint, sess.CloseDone("delegated"))
acceptor := sess.RecvBind(func(ep *sess.Endpoint) kont.Eff[string] {
    return sess.CloseDone("accepted")
})
```

### Pas a Pas

Pour les boucles d'evenements proactor (ex., `io_uring`), `Step` et `Advance` evaluent un effet a la fois. Contrairement a `Run` et `Exec` — qui attendent le progres de maniere synchrone — l'API de stepping retourne `iox.ErrWouldBlock` a l'appelant, permettant a la boucle d'evenements de replanifier.

```go
ep, _ := sess.New()
protocol := sess.ExprSendThen(42, sess.ExprCloseDone[struct{}](struct{}{}))
_, susp := sess.Step[struct{}](protocol)
// In a proactor event loop (e.g., io_uring), yield on boundary:
_, nextSusp, err := sess.Advance(ep, susp)
if err != nil {
    return susp // yield to event loop, reschedule when ready
}
susp = nextSusp
```

### Gestion des Erreurs

Composez des protocoles de session avec des effets d'erreur. `Throw` court-circuite immediatement le protocole et abandonne la suspension en attente.

```go
clientResult, serverResult := sess.RunError[string, string, string](client, server)
// Either[string, string]: Right en cas de succes, Left en cas de Throw
```

## Modele d'Execution

| Fonction | Description |
|----------|-------------------|
| `Run` / `RunExpr` | Executer les deux cotes sur un goroutine — cree une paire d'endpoints en interne |
| `Exec` / `ExecExpr` | Executer un cote sur un endpoint pre-cree |
| `Step` + `Advance` | Evalue un effet a la fois, pour boucles d'evenements externes |

**Cont vs Expr** : Cont est a base de closures et facile a composer. Expr est a base de cadres avec zero allocation amortie, adapte aux chemins critiques.

## API

| Categorie | Cont | Expr |
|-----------|------|------|
| Constructeurs | `SendThen`, `RecvBind`, `CloseDone`, `SelectLThen`, `SelectRThen`, `OfferBranch` | `ExprSendThen`, `ExprRecvBind`, `ExprCloseDone`, `ExprSelectLThen`, `ExprSelectRThen`, `ExprOfferBranch` |
| Recursion | `Loop` | `ExprLoop` |
| Execution | `Exec`, `Run` | `ExecExpr`, `RunExpr` |
| Execution avec erreurs | `ExecError`, `RunError` | `ExecErrorExpr`, `RunErrorExpr` |
| Pas a pas | | `Step`, `Advance`, `StepError`, `AdvanceError` |
| Pont | `Reify` (Cont→Expr), `Reflect` (Expr→Cont) | |
| Transport | `New` → `(*Endpoint, *Endpoint)` | |

## References

- Kohei Honda. "Types for Dyadic Interaction." In *CONCUR 1993* (LNCS 715), pp. 509-523. Springer, 1993. https://doi.org/10.1007/3-540-57208-2_35
- Kohei Honda, Vasco T. Vasconcelos, Makoto Kubo. "Language Primitives and Type Discipline for Structured Communication-Based Programming." In *ESOP 1998* (LNCS 1381), pp. 122-138. Springer, 1998. https://doi.org/10.1007/BFb0053567
- Philip Wadler. "Propositions as Sessions." *Journal of Functional Programming* 24(2-3):384-418, 2014. https://doi.org/10.1017/S095679681400001X
- Dominic A. Orchard, Nobuko Yoshida. "Effects as Sessions, Sessions as Effects." In *POPL 2016*, pp. 568-581. https://doi.org/10.1145/2837614.2837634
- Sam Lindley, J. Garrett Morris. "Lightweight Functional Session Types." In *Behavioural Types: From Theory to Tools*, pp. 265-286, 2017 (first published year; DOI metadata date is 2022-09-01). https://doi.org/10.1201/9781003337331-12
- Simon Fowler, Sam Lindley, J. Garrett Morris, Sara Decova. "Exceptional Asynchronous Session Types: Session Types without Tiers." *Proc. ACM Program. Lang.* 3(POPL):28:1-28:29, 2019. https://doi.org/10.1145/3290341

## Dependances

- [code.hybscloud.com/kont](https://code.hybscloud.com/kont) — Continuations delimitees et effets algebriques
- [code.hybscloud.com/iox](https://code.hybscloud.com/iox) — Semantique non bloquante (`ErrWouldBlock`, `Backoff`)
- [code.hybscloud.com/lfq](https://code.hybscloud.com/lfq) — Files FIFO sans verrou

## Licence

MIT — voir [LICENSE](LICENSE).

©2026 [Hayabusa Cloud Co., Ltd.](https://code.hybscloud.com)
