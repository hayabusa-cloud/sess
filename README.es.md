[![Go Reference](https://pkg.go.dev/badge/code.hybscloud.com/sess.svg)](https://pkg.go.dev/code.hybscloud.com/sess)
[![Benchmarks](https://github.com/hayabusa-cloud/sess/actions/workflows/benchmarks.yml/badge.svg)](https://github.com/hayabusa-cloud/sess/actions/workflows/benchmarks.yml)
[![Coverage Status](https://codecov.io/gh/hayabusa-cloud/sess/graph/badge.svg)](https://codecov.io/gh/hayabusa-cloud/sess)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

[English](README.md) | [简体中文](README.zh-CN.md) | **Español** | [日本語](README.ja.md) | [Français](README.fr.md)

# sess

Los dos participantes deben acordar los tipos de los mensajes y el orden en que los intercambian. En `sess`, los mundos **Cont** y **Expr** ofrecen dos formas equivalentes de escribir en Go el protocolo de cada rol.

## Descripción general

`sess` representa cada acción del protocolo como un efecto algebraico que se ejecuta con [kont](https://code.hybscloud.com/kont). Un rol puede enviar o recibir un valor, elegir una rama o seguir la que eligió el otro participante, y cerrar su extremo de la sesión.

Use **Cont** (`kont.Eff[A]`) cuando quiera dejar los reintentos a cargo del ejecutor. `RecvBind` pasa el valor recibido a una función que devuelve el resto del protocolo del rol. `Run` ejecuta ambos roles. Use `Exec` para un solo extremo si el otro se ejecuta por separado.

Use **Expr** (`kont.Expr[A]`) si el controlador necesita inspeccionar las operaciones o decidir cuándo reintentarlas, como en un bucle de eventos basado en `io_uring`. Los marcos de continuación guardan el trabajo pendiente. `Step` devuelve un resultado o una suspensión, y `Advance` intenta la operación suspendida en el extremo de la sesión. Si `Advance` devuelve `iox.ErrWouldBlock` al intentar recibir, conserve la misma suspensión y reintente la recepción cuando el otro participante haya enviado el valor.

Ambas versiones del mismo rol reciben un `int`, ejecutan `Close` en su endpoint y devuelven el valor recibido. Empareje el receptor con un emisor mediante `Run` en Cont o `RunExpr` en Expr.

```go
contReceiver := sess.RecvBind(sess.CloseDone[int])
exprReceiver := sess.ExprRecvBind(sess.ExprCloseDone[int])
```

## Límite de composición

Los dos extremos intercambian datos y elecciones de rama mediante colas de capacidad limitada. Intentar insertar en una cola llena o extraer de una cola vacía devuelve `iox.ErrWouldBlock`. El controlador puede usar `takt` para asociar las finalizaciones con sus solicitudes, `tempo` para seguir el historial de eventos y `cove` para comprobar el contexto antes de cada intento. El transporte trata `iox.ErrMore` como un error del despachador.

## Instalación

```bash
go get code.hybscloud.com/sess
```

Requiere Go 1.27+.

## Operaciones de sesión

Go comprueba los tipos de los datos y de la respuesta de cada operación. En el protocolo del otro participante, a cada envío le corresponde una recepción del mismo tipo y a cada elección de rama, una oferta. Los datos y las elecciones viajan por colas separadas, por lo que la ejecución puede terminar aunque los roles discrepen en el orden de las operaciones.

| Operación                               | Dual                                 | Contrapresión |
|-----------------------------------------|--------------------------------------|---------------------|
| `Send[T]` — enviar un valor             | `Recv[T]` — recibir un valor         | `iox.ErrWouldBlock` |
| `SelectL` / `SelectR` — elegir una rama | `Offer` — seguir la elección del par | `iox.ErrWouldBlock` |
| `Close` — señalar el cierre local       | `Close`                              | Nunca               |

## Uso

Use `Run` para ejecutar ambos roles en una goroutine, `Exec` para un endpoint gestionado externamente y `Step` con `Advance` cuando el controlador decide cuándo reintentar.

### Envío y recepción

Un envío correcto encola la carga. Si el emisor necesita saber que el otro extremo la ha procesado, haga que este envíe una confirmación después de procesarla.

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

`Step` expone la recepción pendiente. Mientras la cola está vacía, `Advance` devuelve la misma suspensión. Cuando el otro extremo envía el valor, un nuevo intento con esa suspensión lo recibe.

```go
sender, receiver := sess.New()
_, h := sess.Step(kont.ExprPerform(sess.Recv[int]{}))
_, pending, err := sess.Advance(receiver, h)
fmt.Println(iox.IsWouldBlock(err), pending == h) // true true

sess.ExecExpr(sender, kont.ExprPerform(sess.Send[int]{Value: 42}))
value, next, err := sess.Advance(receiver, pending)
fmt.Println(value, next == nil, err) // 42 true <nil>
```

### Manejo de errores

Una ejecución emparejada con manejo de errores termina cuando procesa un `Throw` del tipo esperado que no se ha capturado. Compruebe el valor devuelto en `thrown` antes de interpretar el `Either` del otro extremo.

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

Compare las asignaciones de memoria de protocolos completos y de los reintentos sobre una cola llena. [Guía de benchmarks](BENCHMARKS.md)

## Contrato

`sess` expone una API de transporte orientada a llamadores de confianza. Cada endpoint está pensado para ser usado por una sola goroutine a la vez, y la ruta crítica omite deliberadamente las verificaciones de uso concurrente y las comprobaciones después de `Close`.

Si el tipo de la carga útil es una interfaz, el valor debe seguir portando un tipo dinámico concreto. Los valores de interfaz nil como `any(nil)` o `error(nil)` quedan fuera del contrato; si nil tiene significado semántico, use un valor nil de un tipo concreto o un envoltorio explícito.

Los constructores fusionados, como `ExprSendThen`, crean expresiones de un solo uso. Vuelva a construir el rol para cada ejecución.

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

`RunErrorExpr` crea ambos extremos de la sesión. Cambie la solicitud de `42` a `-1` para leer en `thrown` el motivo por el que el servidor la rechaza.

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

Para recibir datos de un socket en un bucle proactor, cree el par con `sess.New()` y pase a `AdvanceError` cualquier suspensión que devuelva `StepError`. Si la recepción devuelve `iox.ErrWouldBlock`, conserve esa suspensión. Vuelva a intentarlo con ella cuando el adaptador haya enviado el valor decodificado desde el otro extremo.

## Referencias

- Kohei Honda. 1993. Types for Dyadic Interaction. In *Proc. 4th International Conference on Concurrency Theory (CONCUR '93)*. LNCS 715, 509–523. https://doi.org/10.1007/3-540-57208-2_35
- Kohei Honda, Vasco T. Vasconcelos, and Makoto Kubo. 1998. Language Primitives and Type Discipline for Structured Communication-Based Programming. In *Proc. 7th European Symposium on Programming (ESOP '98)*. LNCS 1381, 122–138. https://doi.org/10.1007/BFb0053567
- Philip Wadler. 2014. Propositions as Sessions. *Journal of Functional Programming* 24, 2-3 (2014), 384–418. https://doi.org/10.1017/S095679681400001X
- Dominic A. Orchard and Nobuko Yoshida. 2016. Effects as Sessions, Sessions as Effects. In *Proc. 43rd Annual ACM SIGPLAN-SIGACT Symposium on Principles of Programming Languages (POPL '16)*. 568–581. https://doi.org/10.1145/2837614.2837634
- Sam Lindley and J. Garrett Morris. 2022. Lightweight Functional Session Types. In *Behavioural Types: From Theory to Tools*. 265–286. https://doi.org/10.1201/9781003337331-12
- Simon Fowler, Sam Lindley, J. Garrett Morris, and Sára Decova. 2019. Exceptional Asynchronous Session Types: Session Types without Tiers. *Proc. ACM Program. Lang.* 3, POPL (Jan. 2019), 1–29. https://doi.org/10.1145/3290341

## Dependencias

- [code.hybscloud.com/atomix](https://code.hybscloud.com/atomix) — Identificadores de sesión y contadores de cierre atómicos
- [code.hybscloud.com/kont](https://code.hybscloud.com/kont) — Continuaciones delimitadas y efectos algebraicos
- [code.hybscloud.com/iox](https://code.hybscloud.com/iox) — Semántica no bloqueante (`ErrWouldBlock`, `Backoff`)
- [code.hybscloud.com/lfq](https://code.hybscloud.com/lfq) — Colas FIFO sin bloqueo

## Licencia

MIT — ver [LICENSE](LICENSE).

©2026 [Hayabusa Cloud Co., Ltd.](https://code.hybscloud.com)
