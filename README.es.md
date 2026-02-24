[![Go Reference](https://pkg.go.dev/badge/code.hybscloud.com/sess.svg)](https://pkg.go.dev/code.hybscloud.com/sess)
[![Go Report Card](https://goreportcard.com/badge/github.com/hayabusa-cloud/sess)](https://goreportcard.com/report/github.com/hayabusa-cloud/sess)
[![Coverage Status](https://codecov.io/gh/hayabusa-cloud/sess/graph/badge.svg)](https://codecov.io/gh/hayabusa-cloud/sess)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

[English](README.md) | [简体中文](README.zh-CN.md) | **Español** | [日本語](README.ja.md) | [Français](README.fr.md)

# sess

Protocolos de comunicacion con tipos de sesion via efectos algebraicos sobre [kont](https://code.hybscloud.com/kont).

## Descripcion General

`sess` proporciona protocolos bidireccionales tipados compuestos por seis operaciones, cada una despachada como un efecto algebraico sobre un par de endpoints sin bloqueo.

- Dos estilos de API: Cont (basado en closures) y Expr (basado en marcos, cero asignaciones amortizadas)
- Paso a paso: evalua un efecto a la vez para integracion con proactor y bucle de eventos
- No bloqueante: las operaciones devuelven `iox.ErrWouldBlock` cuando no pueden completarse inmediatamente

## Instalacion

```bash
go get code.hybscloud.com/sess
```

Requiere Go 1.26+.

## Operaciones de Sesion

| Operacion | Efecto | ¿Suspende? |
|-----------|--------|------------|
| `Send[T]` | Enviar un valor | `iox.ErrWouldBlock` |
| `Recv[T]` | Recibir un valor | `iox.ErrWouldBlock` |
| `Close` | Finalizar la sesion | Nunca |
| `SelectL` | Elegir la rama izquierda | `iox.ErrWouldBlock` |
| `SelectR` | Elegir la rama derecha | `iox.ErrWouldBlock` |
| `Offer` | Esperar la eleccion del par | `iox.ErrWouldBlock` |

Delegue un endpoint enviandolo; acepte la delegacion recibiendolo.

## Uso

### Envio y Recepcion

```go
client := sess.SendThen(42, sess.CloseDone("ok"))
server := sess.RecvBind(func(n int) kont.Eff[string] {
    return sess.CloseDone(fmt.Sprintf("got %d", n))
})
a, b := sess.Run(client, server) // "ok", "got 42"
```

Equivalente Expr: `ExprSendThen`, `ExprRecvBind`, `ExprCloseDone`, `RunExpr`.

### Ramificacion

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

```go
counter := sess.Loop(0, func(i int) kont.Eff[kont.Either[int, string]] {
    if i >= 3 {
        return sess.CloseDone(kont.Right[int, string]("done"))
    }
    return sess.SendThen(i, kont.Pure(kont.Left[int, string](i+1)))
})
```

### Paso a Paso

```go
ep, _ := sess.New()
protocol := sess.ExprSendThen(42, sess.ExprCloseDone[struct{}](struct{}{}))
_, susp := sess.Step[struct{}](protocol)
for susp != nil {
    var err error
    _, susp, err = sess.Advance(ep, susp)
    if err != nil {
        continue // reintentar en iox.ErrWouldBlock
    }
}
```

### Manejo de Errores

```go
clientResult, serverResult := sess.RunError[string, string, string](client, server)
// Either[string, string]: Right en exito, Left en Throw
```

## Modelo de Ejecucion

| Funcion | Caso de uso |
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

## Dependencias

- [code.hybscloud.com/kont](https://code.hybscloud.com/kont) — Continuaciones delimitadas y efectos algebraicos
- [code.hybscloud.com/iox](https://code.hybscloud.com/iox) — Semantica no bloqueante (`ErrWouldBlock`, `Backoff`)
- [code.hybscloud.com/lfq](https://code.hybscloud.com/lfq) — Colas FIFO sin bloqueo

## Licencia

MIT — ver [LICENSE](LICENSE).

©2026 [Hayabusa Cloud Co., Ltd.](https://code.hybscloud.com)
