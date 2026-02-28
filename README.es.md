[![Go Reference](https://pkg.go.dev/badge/code.hybscloud.com/sess.svg)](https://pkg.go.dev/code.hybscloud.com/sess)
[![Go Report Card](https://goreportcard.com/badge/github.com/hayabusa-cloud/sess)](https://goreportcard.com/report/github.com/hayabusa-cloud/sess)
[![Coverage Status](https://codecov.io/gh/hayabusa-cloud/sess/graph/badge.svg)](https://codecov.io/gh/hayabusa-cloud/sess)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

[English](README.md) | [简体中文](README.zh-CN.md) | **Español** | [日本語](README.ja.md) | [Français](README.fr.md)

# sess

Protocolos de comunicacion con tipos de sesion via efectos algebraicos sobre [kont](https://code.hybscloud.com/kont).

## Descripcion General

Los tipos de sesion asignan un tipo a cada paso de un protocolo de comunicacion. Cada operacion — enviar, recibir, seleccionar, ofrecer, cerrar — tiene tipos seguros individualmente mediante genericos de Go, y la composicion de protocolos dentro de un mismo endpoint es segura en tipos. La dualidad (correspondencia de operaciones entre endpoints) es responsabilidad del programador: el programador escribe protocolos duales, y las discrepancias se manifiestan en tiempo de ejecucion como fallos de asercion de tipo o deadlocks.

`sess` codifica los tipos de sesion como efectos algebraicos evaluados por el sistema de efectos [kont](https://code.hybscloud.com/kont). Cada paso del protocolo — enviar, recibir, seleccionar, ofrecer, cerrar — es un efecto que suspende la computacion hasta que el transporte completa la operacion. El transporte retorna `iox.ErrWouldBlock` en las fronteras computacionales, permitiendo a los bucles de eventos proactor (ej., `io_uring`) multiplexar la ejecucion sin bloquear hilos.

Dos APIs equivalentes: Cont (basado en closures, composicion directa) y Expr (basado en marcos, cero asignaciones amortizadas para rutas criticas).

## Instalacion

```bash
go get code.hybscloud.com/sess
```

Requiere Go 1.26+.

## Operaciones de Sesion

Cada operacion tiene un dual. Cuando un endpoint realiza una operacion, el otro debe realizar su dual.

| Operacion | Dual | ¿Suspende? |
|-----------|------|------------|
| `Send[T]` — enviar un valor | `Recv[T]` — recibir un valor | `iox.ErrWouldBlock` |
| `SelectL` / `SelectR` — elegir una rama | `Offer` — seguir la eleccion del par | `iox.ErrWouldBlock` |
| `Close` — finalizar la sesion | `Close` | Nunca |

## Uso

Use `Run` para el prototipado y la validación de protocolos. Use `Exec` para endpoints administrados externamente. Use la API Expr (`RunExpr`/`ExecExpr`) para el control de stepping o para minimizar la sobrecarga de asignación en rutas críticas.

### Envio y Recepcion

Un lado envia un valor; el lado dual lo recibe.

```go
client := sess.SendThen(42, sess.CloseDone("ok"))
server := sess.RecvBind(func(n int) kont.Eff[string] {
    return sess.CloseDone(fmt.Sprintf("got %d", n))
})
a, b := sess.Run(client, server) // "ok", "got 42"
```

Equivalente Expr: `ExprSendThen`, `ExprRecvBind`, `ExprCloseDone`, `RunExpr`.

### Ramificacion

Un lado selecciona una rama; el lado dual ofrece ambas ramas y sigue la seleccion.

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

### Protocolos Recursivos

Los protocolos que se repiten usan `Loop` con `Either`: `Left` continua el bucle, `Right` termina.

```go
counter := sess.Loop(0, func(i int) kont.Eff[kont.Either[int, string]] {
    if i >= 3 {
        return sess.CloseDone(kont.Right[int, string]("done"))
    }
    return sess.SendThen(i, kont.Pure(kont.Left[int, string](i+1)))
})
```

### Delegacion

Transfiera un endpoint a un tercero enviandolo; acepte la delegacion recibiendolo.

```go
delegator := sess.SendThen(endpoint, sess.CloseDone("delegated"))
acceptor := sess.RecvBind(func(ep *sess.Endpoint) kont.Eff[string] {
    return sess.CloseDone("accepted")
})
```

### Paso a Paso

Para bucles de eventos proactor (ej., `io_uring`), `Step` y `Advance` evaluan un efecto a la vez. A diferencia de `Run` y `Exec` — que esperan sincronamente el progreso — la API de stepping devuelve `iox.ErrWouldBlock` al llamador, permitiendo al bucle de eventos reprogramar.

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

### Manejo de Errores

Componga protocolos de sesion con efectos de error. `Throw` cortocircuita el protocolo y descarta la suspension pendiente.

```go
clientResult, serverResult := sess.RunError[string, string, string](client, server)
// Either[string, string]: Right en exito, Left en Throw
```

## Modelo de Ejecucion

| Funcion | Descripcion |
|---------|-------------|
| `Run` / `RunExpr` | Ejecutar ambos lados en un goroutine — crea un par de endpoints internamente |
| `Exec` / `ExecExpr` | Ejecutar un lado en un endpoint pre-creado |
| `Step` + `Advance` | Evalua un efecto a la vez, para bucles de eventos externos |

**Cont vs Expr**: Cont se basa en closures y es sencillo de componer. Expr se basa en marcos con cero asignaciones amortizadas, adecuado para rutas criticas.

## API

| Categoria | Cont | Expr |
|-----------|------|------|
| Constructores | `SendThen`, `RecvBind`, `CloseDone`, `SelectLThen`, `SelectRThen`, `OfferBranch` | `ExprSendThen`, `ExprRecvBind`, `ExprCloseDone`, `ExprSelectLThen`, `ExprSelectRThen`, `ExprOfferBranch` |
| Recursion | `Loop` | `ExprLoop` |
| Ejecucion | `Exec`, `Run` | `ExecExpr`, `RunExpr` |
| Ejecucion con errores | `ExecError`, `RunError` | `ExecErrorExpr`, `RunErrorExpr` |
| Paso a paso | | `Step`, `Advance`, `StepError`, `AdvanceError` |
| Puente | `Reify` (Cont→Expr), `Reflect` (Expr→Cont) | |
| Transporte | `New` → `(*Endpoint, *Endpoint)` | |

## References

- Kohei Honda. "Types for Dyadic Interaction." In *CONCUR 1993* (LNCS 715), pp. 509-523. Springer, 1993. https://doi.org/10.1007/3-540-57208-2_35
- Kohei Honda, Vasco T. Vasconcelos, Makoto Kubo. "Language Primitives and Type Discipline for Structured Communication-Based Programming." In *ESOP 1998* (LNCS 1381), pp. 122-138. Springer, 1998. https://doi.org/10.1007/BFb0053567
- Philip Wadler. "Propositions as Sessions." *Journal of Functional Programming* 24(2-3):384-418, 2014. https://doi.org/10.1017/S095679681400001X
- Dominic A. Orchard, Nobuko Yoshida. "Effects as Sessions, Sessions as Effects." In *POPL 2016*, pp. 568-581. https://doi.org/10.1145/2837614.2837634
- Sam Lindley, J. Garrett Morris. "Lightweight Functional Session Types." In *Behavioural Types: From Theory to Tools*, pp. 265-286, 2017 (first published year; DOI metadata date is 2022-09-01). https://doi.org/10.1201/9781003337331-12
- Simon Fowler, Sam Lindley, J. Garrett Morris, Sara Decova. "Exceptional Asynchronous Session Types: Session Types without Tiers." *Proc. ACM Program. Lang.* 3(POPL):28:1-28:29, 2019. https://doi.org/10.1145/3290341

## Dependencias

- [code.hybscloud.com/kont](https://code.hybscloud.com/kont) — Continuaciones delimitadas y efectos algebraicos
- [code.hybscloud.com/iox](https://code.hybscloud.com/iox) — Semantica no bloqueante (`ErrWouldBlock`, `Backoff`)
- [code.hybscloud.com/lfq](https://code.hybscloud.com/lfq) — Colas FIFO sin bloqueo

## Licencia

MIT — ver [LICENSE](LICENSE).

©2026 [Hayabusa Cloud Co., Ltd.](https://code.hybscloud.com)
