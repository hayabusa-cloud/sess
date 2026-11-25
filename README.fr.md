[![Go Reference](https://pkg.go.dev/badge/code.hybscloud.com/sess.svg)](https://pkg.go.dev/code.hybscloud.com/sess)
[![Benchmarks](https://github.com/hayabusa-cloud/sess/actions/workflows/benchmarks.yml/badge.svg)](https://github.com/hayabusa-cloud/sess/actions/workflows/benchmarks.yml)
[![Coverage Status](https://codecov.io/gh/hayabusa-cloud/sess/graph/badge.svg)](https://codecov.io/gh/hayabusa-cloud/sess)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

[English](README.md) | [简体中文](README.zh-CN.md) | [Español](README.es.md) | [日本語](README.ja.md) | **Français**

# sess

Les deux participants doivent s’accorder sur les types des messages et leur ordre. Dans `sess`, les mondes **Cont** et **Expr** offrent deux façons équivalentes d’écrire en Go le protocole de chaque rôle.

## Présentation

`sess` représente chaque action du protocole par un effet algébrique exécuté avec [kont](https://code.hybscloud.com/kont). Un rôle peut envoyer ou recevoir une valeur, choisir une branche ou suivre le choix de l’autre participant, et fermer son extrémité de la session.

Utilisez **Cont** (`kont.Eff[A]`) pour confier les nouvelles tentatives au moteur d’exécution. `RecvBind` transmet la valeur reçue à une fonction qui renvoie la suite du protocole. `Run` exécute les deux rôles. Utilisez `Exec` pour un seul endpoint si l’autre s’exécute séparément.

Utilisez **Expr** (`kont.Expr[A]`) si le pilote doit examiner les opérations ou décider quand les retenter, comme dans une boucle d’événements fondée sur `io_uring`. Les cadres de continuation conservent la suite du calcul. `Step` renvoie un résultat ou une suspension, et `Advance` tente l’opération suspendue sur l’endpoint. Si `Advance` renvoie `iox.ErrWouldBlock` lors d’une réception, conservez la même suspension et réessayez après l’envoi du message par l’autre participant.

Ces deux versions du même rôle reçoivent un `int`, exécutent `Close` localement et renvoient la valeur reçue. Associez le récepteur à un émetteur avec `Run` dans le monde Cont ou `RunExpr` dans le monde Expr.

```go
contReceiver := sess.RecvBind(sess.CloseDone[int])
exprReceiver := sess.ExprRecvBind(sess.ExprCloseDone[int])
```

## Limite de composition

Les deux extrémités échangent les données et les choix de branche dans des files de capacité limitée. Une tentative d’ajout dans une file pleine ou de retrait dans une file vide renvoie `iox.ErrWouldBlock`. Un pilote peut utiliser `takt` pour associer les complétions aux requêtes, `tempo` pour suivre l’historique des événements et `cove` pour vérifier le contexte avant chaque tentative. Le transport traite `iox.ErrMore` comme une erreur du répartiteur.

## Installation

```bash
go get code.hybscloud.com/sess
```

Nécessite Go 1.27+.

## Opérations de session

Go vérifie les types des données et de la réponse de chaque opération. Dans le protocole de l’autre participant, faites correspondre à chaque envoi une réception du même type et à chaque choix de branche une offre. Les données et les choix empruntent des files distinctes, si bien que l’exécution peut se terminer même si les deux rôles ne suivent pas le même ordre.

| Opération                                   | Duale                             | Contre-pression |
|---------------------------------------------|-----------------------------------|---------------------|
| `Send[T]` — envoyer une valeur              | `Recv[T]` — recevoir une valeur   | `iox.ErrWouldBlock` |
| `SelectL` / `SelectR` — choisir une branche | `Offer` — suivre le choix du pair | `iox.ErrWouldBlock` |
| `Close` — signaler la fermeture locale      | `Close`                           | Jamais              |

## Utilisation

Utilisez `Run` pour exécuter les deux rôles dans une goroutine, `Exec` pour un endpoint géré en externe et `Step` avec `Advance` lorsque le pilote décide quand réessayer.

### Envoi et réception

Un envoi réussi place les données dans la file. Si l’émetteur doit savoir que le pair les a traitées, faites envoyer un accusé de réception par le pair après le traitement.

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

`Step` expose la réception en attente. Tant que la file est vide, `Advance` renvoie la même suspension. Une fois l’envoi effectué par le pair, réessayez avec cette suspension pour obtenir la valeur.

```go
sender, receiver := sess.New()
_, h := sess.Step(kont.ExprPerform(sess.Recv[int]{}))
_, pending, err := sess.Advance(receiver, h)
fmt.Println(iox.IsWouldBlock(err), pending == h) // true true

sess.ExecExpr(sender, kont.ExprPerform(sess.Send[int]{Value: 42}))
value, next, err := sess.Advance(receiver, pending)
fmt.Println(value, next == nil, err) // 42 true <nil>
```

### Gestion des erreurs

Une exécution appariée avec gestion des erreurs prend fin lorsqu’elle traite un `Throw` correspondant et non intercepté. Vérifiez la valeur renvoyée dans `thrown` avant d’interpréter le `Either` du pair.

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
- `thrown != nil` : l'exécution appariée a été interrompue globalement ; `*thrown` est le `Throw` non capturé et le `Either` du pair peut encore être non résolu.

## Modèle d'exécution

| Fonction            | Description                                                                      |
|---------------------|----------------------------------------------------------------------------------|
| `Run` / `RunExpr`   | Exécute les deux côtés sur une goroutine — crée une paire d'endpoints en interne |
| `Exec` / `ExecExpr` | Exécute un côté sur un endpoint pré-créé                                         |
| `Step` + `Advance`  | Évalue un effet à la fois, pour boucles d'événements externes                    |

Comparez les allocations pour des protocoles complets et pour une nouvelle tentative sur une file pleine. [Guide des benchmarks](BENCHMARKS.md)

## Contrat

`sess` expose une API de transport destinée à des appelants de confiance. Chaque endpoint est conçu pour être utilisé par une seule goroutine à la fois, et le chemin critique omet délibérément les contrôles d'usage concurrent ainsi que les vérifications après `Close`.

Si le type de la charge utile est une interface, la valeur doit tout de même porter un type dynamique concret. Les valeurs d'interface nil comme `any(nil)` ou `error(nil)` sont hors contrat ; si nil a une signification sémantique, utilisez une valeur nil d'un type concret ou un wrapper explicite.

Les constructeurs fusionnés tels que `ExprSendThen` créent des expressions à usage unique. Reconstruisez le rôle à chaque exécution.

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

`RunErrorExpr` crée les deux extrémités de la session. Remplacez `42` par `-1` dans la requête pour lire dans `thrown` le motif du refus du serveur.

```go
type RequestError struct{ Reason string }

client := sess.ExprSendThen(42, sess.ExprRecvBind(sess.ExprCloseDone[string]))
server := sess.ExprRecvBind(func(n int) kont.Expr[string] {
	if n < 0 {
		return kont.ExprThrowError[RequestError, string](RequestError{Reason: "negative request"})
	}
	return sess.ExprSendThen(fmt.Sprintf("got %d", n), sess.ExprCloseDone("ok"))
})

clientResult, serverResult, thrown := sess.RunErrorExpr[RequestError](client, server)
if thrown != nil {
	fmt.Println(thrown.Reason)
	return
}
reply, _ := clientResult.GetRight()
status, _ := serverResult.GetRight()
fmt.Println(reply, status) // got 42 ok
```

Pour traiter une lecture réseau dans une boucle proactor, créez la paire avec `sess.New()` et passez à `AdvanceError` toute suspension renvoyée par `StepError`. Si la réception renvoie `iox.ErrWouldBlock`, conservez cette suspension. Réessayez avec elle une fois que l’adaptateur a envoyé la valeur décodée depuis l’autre extrémité.

## Références

- Kohei Honda. 1993. Types for Dyadic Interaction. In *Proc. 4th International Conference on Concurrency Theory (CONCUR '93)*. LNCS 715, 509–523. https://doi.org/10.1007/3-540-57208-2_35
- Kohei Honda, Vasco T. Vasconcelos, and Makoto Kubo. 1998. Language Primitives and Type Discipline for Structured Communication-Based Programming. In *Proc. 7th European Symposium on Programming (ESOP '98)*. LNCS 1381, 122–138. https://doi.org/10.1007/BFb0053567
- Philip Wadler. 2014. Propositions as Sessions. *Journal of Functional Programming* 24, 2-3 (2014), 384–418. https://doi.org/10.1017/S095679681400001X
- Dominic A. Orchard and Nobuko Yoshida. 2016. Effects as Sessions, Sessions as Effects. In *Proc. 43rd Annual ACM SIGPLAN-SIGACT Symposium on Principles of Programming Languages (POPL '16)*. 568–581. https://doi.org/10.1145/2837614.2837634
- Sam Lindley and J. Garrett Morris. 2022. Lightweight Functional Session Types. In *Behavioural Types: From Theory to Tools*. 265–286. https://doi.org/10.1201/9781003337331-12
- Simon Fowler, Sam Lindley, J. Garrett Morris, and Sára Decova. 2019. Exceptional Asynchronous Session Types: Session Types without Tiers. *Proc. ACM Program. Lang.* 3, POPL (Jan. 2019), 1–29. https://doi.org/10.1145/3290341

## Dépendances

- [code.hybscloud.com/atomix](https://code.hybscloud.com/atomix) — Identifiants de session et compteurs de fermeture atomiques
- [code.hybscloud.com/kont](https://code.hybscloud.com/kont) — Continuations délimitées et effets algébriques
- [code.hybscloud.com/iox](https://code.hybscloud.com/iox) — Sémantique non bloquante (`ErrWouldBlock`, `Backoff`)
- [code.hybscloud.com/lfq](https://code.hybscloud.com/lfq) — Files FIFO sans verrou

## Licence

MIT — voir [LICENSE](LICENSE).

©2026 [Hayabusa Cloud Co., Ltd.](https://code.hybscloud.com)
