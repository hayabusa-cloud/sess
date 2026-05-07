[![Go Reference](https://pkg.go.dev/badge/code.hybscloud.com/sess.svg)](https://pkg.go.dev/code.hybscloud.com/sess)
[![Go Report Card](https://goreportcard.com/badge/github.com/hayabusa-cloud/sess)](https://goreportcard.com/report/github.com/hayabusa-cloud/sess)
[![Coverage Status](https://codecov.io/gh/hayabusa-cloud/sess/graph/badge.svg)](https://codecov.io/gh/hayabusa-cloud/sess)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

[English](README.md) | [简体中文](README.zh-CN.md) | **Español** | [日本語](README.ja.md) | [Français](README.fr.md)

# sess

Protocolos de comunicación con tipos de sesión mediante efectos algebraicos sobre [kont](https://code.hybscloud.com/kont).

## Descripción general

Los tipos de sesión asignan un tipo a cada paso de un protocolo de comunicación. Cada operación —enviar, recibir, seleccionar, ofrecer, cerrar— está individualmente bien tipada gracias a los genéricos de Go, y la composición de protocolos dentro de un mismo endpoint es segura en tipos. La dualidad (la correspondencia de operaciones entre endpoints) es responsabilidad del programador: es él quien escribe los protocolos duales y las discrepancias se manifiestan en tiempo de ejecución como fallos de aserción de tipo o interbloqueos.

`sess` codifica los tipos de sesión como efectos algebraicos evaluados por el sistema de efectos [kont](https://code.hybscloud.com/kont). Cada paso del protocolo —enviar, recibir, seleccionar, ofrecer, cerrar— es un efecto que suspende la computación hasta que el transporte completa la operación. El transporte devuelve `iox.ErrWouldBlock` en las fronteras de cómputo, lo que permite a los bucles de eventos proactor (p. ej. `io_uring`) multiplexar la ejecución sin bloquear hilos.

Hay dos familias de API equivalentes: Cont (basada en closures, fácil de componer) y Expr (basada en marcos, con cero asignaciones amortizadas en las rutas críticas).

## Límite de composición

`sess` es dueño de la signatura de efectos de sesión y del transporte de endpoints que la interpreta. Usa `iox.ErrWouldBlock` como frontera no bloqueante para colas acotadas, pero no posee el álgebra completa de resultados de `iox`; `takt` posee la planificación de estilo proactor y la correlación de finalizaciones; `cove` posee la evidencia contextual para composición alrededor de suspensiones. Dentro de `sess`, `iox.ErrMore` queda fuera del dominio de transporte de endpoints y se trata como un fallo inesperado del despachador.

## Instalación

```bash
go get code.hybscloud.com/sess
```

Requiere Go 1.26+.

## Operaciones de sesión

Cada operación tiene un dual. Cuando un endpoint realiza una operación, el otro debe realizar su dual.

| Operación                               | Dual                                 | ¿Suspende?          |
|-----------------------------------------|--------------------------------------|---------------------|
| `Send[T]` — enviar un valor             | `Recv[T]` — recibir un valor         | `iox.ErrWouldBlock` |
| `SelectL` / `SelectR` — elegir una rama | `Offer` — seguir la elección del par | `iox.ErrWouldBlock` |
| `Close` — finalizar la sesión           | `Close`                              | Nunca               |

## Uso

Use `Run` para prototipar y validar protocolos. Use `Exec` con endpoints administrados externamente. Use la API Expr (`RunExpr`/`ExecExpr`) cuando necesite control paso a paso o quiera minimizar la sobrecarga de asignación en rutas críticas.

### Envío y recepción

Un lado envía un valor; el lado dual lo recibe.

```go
client := sess.SendThen(42, sess.CloseDone("ok"))
server := sess.RecvBind(func(n int) kont.Eff[string] {
	return sess.CloseDone(fmt.Sprintf("got %d", n))
})
a, b := sess.Run(client, server) // "ok", "got 42"
```

Equivalente Expr: `ExprSendThen`, `ExprRecvBind`, `ExprCloseDone`, `RunExpr`.

### Ramificación

Un lado selecciona una rama; el lado dual ofrece ambas ramas y sigue la selección.

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

### Protocolos recursivos

Los protocolos repetitivos usan `Loop` con `Either`: `Left` continúa el bucle, `Right` lo termina.

```go
counter := sess.Loop(0, func(i int) kont.Eff[kont.Either[int, string]] {
	if i >= 3 {
		return sess.CloseDone(kont.Right[int, string]("done"))
	}
	return sess.SendThen(i, kont.Pure(kont.Left[int, string](i+1)))
})
```

### Delegación

Transfiera un endpoint a un tercero enviándolo; acepte la delegación recibiéndolo.

```go
delegator := sess.SendThen(endpoint, sess.CloseDone("delegated"))
acceptor := sess.RecvBind(func(ep *sess.Endpoint) kont.Eff[string] {
	return sess.CloseDone("accepted")
})
```

### Paso a paso

Para bucles de eventos proactor (p. ej. `io_uring`), `Step` y `Advance` evalúan un efecto a la vez. A diferencia de `Run` y `Exec` —que esperan los progresos de forma síncrona—, la API de stepping devuelve `iox.ErrWouldBlock` al llamador, permitiendo al bucle de eventos reprogramar.

```go
ep, _ := sess.New()
protocol := sess.ExprSendThen(42, sess.ExprCloseDone[struct{}](struct{}{}))
_, susp := sess.Step[struct{}](protocol)
// En un bucle de eventos proactor (p. ej. io_uring), ceder en la frontera:
_, nextSusp, err := sess.Advance(ep, susp)
if err != nil {
	return susp // ceder al bucle de eventos; reprogramar cuando esté listo
}
susp = nextSusp
```

### Manejo de errores

Componga protocolos de sesión con efectos de error. `Throw` aborta de inmediato la ejecución emparejada. El valor `thrown` devuelto es la causa global de un `Throw` no capturado; revíselo antes de interpretar el `Either` del par.

```go
client := kont.ExprThrowError[string, string]("boom")
server := sess.ExprRecvBind(func(v string) kont.Expr[string] {
	return sess.ExprCloseDone("recv: " + v)
})

clientResult, serverResult, thrown := sess.RunErrorExpr[string](client, server)
if thrown != nil {
	// Aborto global de la sesión.
	fmt.Println("session aborted:", *thrown)
	// El Either del par puede seguir aún sin resolverse localmente.
	_ = clientResult
	_ = serverResult
	return
}

// Sin Throw global no capturado: ambos Either son resultados locales finales.
fmt.Println(clientResult, serverResult)
```

En resumen:

- `thrown == nil`: ambos valores `Either` son resultados locales finales.
- `thrown != nil`: la ejecución emparejada se abortó globalmente; `*thrown` es el `Throw` no capturado y el `Either` del par puede seguir aún sin resolverse.

## Modelo de ejecución

| Función             | Descripción                                                                  |
|---------------------|------------------------------------------------------------------------------|
| `Run` / `RunExpr`   | Ejecuta ambos lados en una goroutine — crea internamente un par de endpoints |
| `Exec` / `ExecExpr` | Ejecuta un lado sobre un endpoint precreado                                  |
| `Step` + `Advance`  | Evalúa un efecto a la vez, para bucles de eventos externos                   |

**Cont vs Expr**: Cont se basa en closures y es sencillo de componer. Expr se basa en marcos con cero asignaciones amortizadas, adecuado para rutas críticas.

## Contrato

`sess` expone una API de transporte orientada a llamadores de confianza. Cada endpoint está pensado para ser usado por una sola goroutine a la vez, y la ruta crítica omite deliberadamente las verificaciones de uso concurrente y las comprobaciones después de `Close`.

Si el tipo de la carga útil es una interfaz, el valor debe seguir portando un tipo dinámico concreto. Los valores de interfaz nil como `any(nil)` o `error(nil)` quedan fuera del contrato; si nil tiene significado semántico, use un valor nil de un tipo concreto o un envoltorio explícito.

## API

| Categoría             | Cont                                                                             | Expr                                                                                                     |
|-----------------------|----------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------|
| Constructores         | `SendThen`, `RecvBind`, `CloseDone`, `SelectLThen`, `SelectRThen`, `OfferBranch` | `ExprSendThen`, `ExprRecvBind`, `ExprCloseDone`, `ExprSelectLThen`, `ExprSelectRThen`, `ExprOfferBranch` |
| Recursión             | `Loop`                                                                           | `ExprLoop`                                                                                               |
| Ejecución             | `Exec`, `Run`                                                                    | `ExecExpr`, `RunExpr`                                                                                    |
| Ejecución con errores | `ExecError`, `RunError`                                                          | `ExecErrorExpr`, `RunErrorExpr`                                                                          |
| Paso a paso           |                                                                                  | `Step`, `Advance`, `StepError`, `AdvanceError`                                                           |
| Puente                | `Reify` (Cont→Expr), `Reflect` (Expr→Cont)                                       |                                                                                                          |
| Transporte            | `New` → `(*Endpoint, *Endpoint)`                                                 |                                                                                                          |

## Patrones prácticos

Una ejecución emparejada con manejo de errores define protocolos duales y deja que `RunErrorExpr` cree internamente el par de endpoints:

```go
// 1. Definir el protocolo en cada lado usando las operaciones duales.
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

// 2. Avanzar ambos lados al unísono con manejo de errores.
type Err struct{ Reason string }
left, right, thrown := sess.RunErrorExpr[Err](clientProg, serverProg)
if thrown != nil {
	// La sesión se abortó; left/right pueden contener solo resultados parciales.
	_ = thrown
}
_ = left; _ = right
```

Para integración con un proactor, cree endpoints con `sess.New()` y conduzca `StepError` / `AdvanceError` directamente: la suspensión cede cada vez que el transporte subyacente devuelve `iox.ErrWouldBlock`, y el bucle la reanuda cuando el endpoint correspondiente completa.

## Referencias

- Kohei Honda. 1993. Types for Dyadic Interaction. In *Proc. 4th International Conference on Concurrency Theory (CONCUR '93)*. LNCS 715, 509–523. https://doi.org/10.1007/3-540-57208-2_35
- Kohei Honda, Vasco T. Vasconcelos, and Makoto Kubo. 1998. Language Primitives and Type Discipline for Structured Communication-Based Programming. In *Proc. 7th European Symposium on Programming (ESOP '98)*. LNCS 1381, 122–138. https://doi.org/10.1007/BFb0053567
- Philip Wadler. 2014. Propositions as Sessions. *Journal of Functional Programming* 24, 2-3 (2014), 384–418. https://doi.org/10.1017/S095679681400001X
- Dominic A. Orchard and Nobuko Yoshida. 2016. Effects as Sessions, Sessions as Effects. In *Proc. 43rd Annual ACM SIGPLAN-SIGACT Symposium on Principles of Programming Languages (POPL '16)*. 568–581. https://doi.org/10.1145/2837614.2837634
- Sam Lindley and J. Garrett Morris. 2022. Lightweight Functional Session Types. In *Behavioural Types: From Theory to Tools*. 265–286. https://doi.org/10.1201/9781003337331-12
- Simon Fowler, Sam Lindley, J. Garrett Morris, and Sára Decova. 2019. Exceptional Asynchronous Session Types: Session Types without Tiers. *Proc. ACM Program. Lang.* 3, POPL (Jan. 2019), 1–29. https://doi.org/10.1145/3290341

## Dependencias

- [code.hybscloud.com/kont](https://code.hybscloud.com/kont) — Continuaciones delimitadas y efectos algebraicos
- [code.hybscloud.com/iox](https://code.hybscloud.com/iox) — Semántica no bloqueante (`ErrWouldBlock`, `Backoff`)
- [code.hybscloud.com/lfq](https://code.hybscloud.com/lfq) — Colas FIFO sin bloqueo

## Licencia

MIT — ver [LICENSE](LICENSE).

©2026 [Hayabusa Cloud Co., Ltd.](https://code.hybscloud.com)
