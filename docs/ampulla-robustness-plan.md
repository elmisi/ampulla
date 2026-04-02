# Plan: Robustezza di Ampulla - no-loss ingest + ntfy shared config

**STATO: COMPLETATO** — implementato e deployato in v0.5.0 (2026-04-02).

Resta da fare solo la migrazione 010 (rimozione colonne legacy `ntfy_*` da projects) dopo verifica funzionale prolungata.

## Summary

Obiettivo di questa iterazione:

1. **Non perdere eventi in ingest** quando la coda e' piena o il servizio e' in shutdown.
2. **Rendere visibili gli errori interni di Ampulla** tramite `slog` sempre e Sentry best-effort.
3. **Rendere `ntfy` affidabile e manutenibile** spostando la configurazione fuori dai progetti in un modello globale riusabile.

Questo piano e' pensato per essere **eseguibile senza ulteriore contesto**. Tutte le decisioni funzionali e tecniche necessarie sono fissate qui.

Stato attuale del repository, su cui il piano si basa:

- `Processor.Enqueue()` scarta eventi quando la coda e' piena e gli endpoint ingest rispondono comunque `200`.
- `sendNtfy()` e' fire-and-forget, non tracciato nel `WaitGroup`, e fallisce in modo poco osservabile.
- la configurazione `ntfy` oggi vive inline su `projects` (`ntfy_url`, `ntfy_topic`, `ntfy_token`).
- il frontend admin supporta Sentry **solo se** `SENTRY_FRONTEND_DSN` e' configurato; questo piano copre il backend Go.

## Decisions Fixed

### Ingest / no-loss

- `Processor.Enqueue(...)` cambia firma e ritorna `bool`.
- Se la coda e' piena, ingest risponde `503 Service Unavailable` con header `Retry-After: 60`.
- Non si usa `429`.
- Non si usa blocking enqueue con timeout lato HTTP.

### Self-monitoring

- `slog` e' la fonte minima garantita per ogni errore interno.
- Sentry backend e' **best-effort** perche' Ampulla si auto-monitora e puo' essere degradata proprio quando deve auto-inviarsi eventi.
- I queue drop non generano un evento Sentry per ogni drop: si usa aggregazione throttled.

### ntfy

- Le configurazioni `ntfy` diventano **globali**.
- Ogni progetto seleziona **una configurazione opzionale** oppure `none`.
- La migrazione dal modello inline attuale e' **automatica**.
- La deduplica usa il tripletto completo `(url, topic, token)`.
- Il test canonico e' sulla **configurazione ntfy**, non sul progetto.

### Rollout

- La migrazione schema e dati `009` crea il nuovo modello e popola `projects.ntfy_config_id`.
- Il codice applicativo passa a leggere/scrivere solo il nuovo modello nella stessa iterazione.
- Le colonne legacy `projects.ntfy_*` vengono rimosse solo in una migrazione successiva `010`, dopo verifica funzionale.

## Implementation Plan

### 1. Ingest durability

**Problema da risolvere**

Gli handler in `internal/api/ingest/handler.go` chiamano `Processor.Enqueue()` e rispondono sempre `200`, anche quando `Processor.Enqueue()` scarta l'evento per coda piena.

**Modifica richiesta**

- Cambiare `Processor.Enqueue(projectID int64, env *Envelope, sdkClient string)` in:

```go
func (p *Processor) Enqueue(projectID int64, env *Envelope, sdkClient string) bool
```

- `true` significa job accettato in coda.
- `false` significa job rifiutato per overload.

**Comportamento handler ingest**

- `Envelope()` e `Store()` devono controllare il ritorno di `Enqueue`.
- Se `false`:
  - `Retry-After: 60`
  - `503 Service Unavailable`
  - body JSON: `{"error":"server overloaded"}`
- Se `true`, il comportamento di risposta resta invariato.

**Log e metriche**

- Ad ogni drop:
  - `slog.Warn("event queue full, dropping event", ...)`
  - incremento di `queueDropCount`
- L'evento Sentry su queue drop e' aggregato tramite throttling, non per-evento.

### 2. Self-monitoring backend

**Nuovo package**

Creare `internal/observe/capture.go`.

**API fissate**

```go
func Error(ctx context.Context, msg string, err error, attrs ...any)
func Message(ctx context.Context, level slog.Level, msg string, attrs ...any)
func RecoverPanic(ctx context.Context, where string, attrs ...any)
func Throttled(key string, interval time.Duration, fn func())
```

**Semantica**

- `Error(...)`
  - logga sempre via `slog.Error`
  - cattura via `sentry.CaptureException` best-effort
- `Message(...)`
  - logga via `slog`
  - cattura via `sentry.CaptureMessage` best-effort
- `RecoverPanic(...)`
  - va usata come `defer observe.RecoverPanic(ctx, "worker", "project", projectID)`
  - fa internamente `recover()`
  - logga panic + stack trace completo (`runtime/debug.Stack()`)
  - invia a Sentry con `sentry.CurrentHub().RecoverWithContext(...)` best-effort
- `Throttled(...)`
  - esegue `fn` al massimo una volta per chiave e intervallo
  - implementazione thread-safe

**Errori da coprire**

- queue drop
- panic worker
- panic goroutine ntfy
- cleanup failure
- `serverError` admin
- startup failure
- shutdown failure
- panic HTTP globale
- `ntfy` HTTP error `>= 500`

**Queue drop aggregation**

- Aggiungere in `Processor`:
  - `queueDropCount atomic.Int64`
- Ad ogni drop incrementare il contatore.
- Con `observe.Throttled("queue_full", time.Minute, ...)` inviare al massimo un messaggio Sentry al minuto con il numero di drop accumulati nel periodo.
- Il log locale resta per ogni singolo drop.

**Middleware panic HTTP**

- Aggiungere un middleware globale prima di `middleware.Recoverer`.
- Il middleware deve:
  - fare `defer observe.RecoverPanic(r.Context(), "http", "path", r.URL.Path, "method", r.Method)`
  - lasciare a `middleware.Recoverer` il compito di trasformare il panic in `500`
- Deve essere applicato al router principale, non solo ad admin/web.

### 3. ntfy shared configuration model

**Problema da risolvere**

La configurazione `ntfy` inline su `projects` duplica dati, rende fragile la rotazione dei token e complica il debugging.

**Nuovo modello**

Creare tabella `ntfy_configurations`:

```sql
CREATE TABLE ntfy_configurations (
    id         BIGSERIAL PRIMARY KEY,
    name       TEXT NOT NULL,
    url        TEXT NOT NULL,
    topic      TEXT NOT NULL,
    token      TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

Creare indice unico logico sul tripletto:

```sql
CREATE UNIQUE INDEX idx_ntfy_configurations_unique_triplet
ON ntfy_configurations (url, topic, COALESCE(token, ''));
```

Aggiungere al progetto:

```sql
ALTER TABLE projects
ADD COLUMN ntfy_config_id BIGINT REFERENCES ntfy_configurations(id) ON DELETE SET NULL;
```

**Regola funzionale**

- `projects.ntfy_config_id = NULL` significa `ntfy` disabilitato per quel progetto.
- Ogni progetto ha al massimo una configurazione `ntfy`.

**Migrazione 009**

Passi:

1. creare `ntfy_configurations`
2. creare `projects.ntfy_config_id`
3. leggere tutti i progetti con `ntfy_url`, `ntfy_topic`, `ntfy_token`
4. per ogni tripletto distinto `(url, topic, token)` creare una configurazione se non esiste gia'
5. assegnare `ntfy_config_id` ai progetti corrispondenti
6. lasciare intatte le colonne legacy `projects.ntfy_*`

**Naming automatico config migrate**

- default: `topic`
- se il nome collide, usare `topic (2)`, `topic (3)`, ecc.

**Migrazione 010**

Da fare solo dopo verifica funzionale:

```sql
ALTER TABLE projects DROP COLUMN ntfy_url;
ALTER TABLE projects DROP COLUMN ntfy_topic;
ALTER TABLE projects DROP COLUMN ntfy_token;
```

### 4. Service condiviso per invio ntfy

**Decisione strutturale**

La logica di invio `ntfy` non resta dentro `admin.Handler` o `event.Processor`. Va estratta in un servizio condiviso.

Creare `internal/notify/ntfy.go`.

**Interfacce**

```go
type NtfyConfig struct {
    ID    int64
    Name  string
    URL   string
    Topic string
    Token string
}

type NtfyPayload struct {
    Title    string
    Body     string
    ClickURL string
}

type NtfySender interface {
    Send(ctx context.Context, cfg NtfyConfig, payload NtfyPayload) (statusCode int, responseBody string, err error)
}
```

**Implementazione**

- usare `http.Client` dedicato
- configurare `Transport` esplicito
- usare `context.WithTimeout(10 * time.Second)` lato chiamante
- ritornare sempre `statusCode`, `responseBody`, `err`

**Uso**

- `event.Processor` usa `NtfySender` per gli invii reali
- `admin.Handler` usa lo stesso `NtfySender` per il test

### 5. Lettura configurazione ntfy dal progetto

**Store**

Cambiare `GetProjectNtfyConfig(...)`.

Firma fissata:

```go
func (db *DB) GetProjectNtfyConfig(ctx context.Context, projectID int64) (projectName string, cfg *notify.NtfyConfig, err error)
```

**Query**

Usare `LEFT JOIN`:

```sql
SELECT p.name, nc.id, nc.name, nc.url, nc.topic, nc.token
FROM projects p
LEFT JOIN ntfy_configurations nc ON nc.id = p.ntfy_config_id
WHERE p.id = $1
```

**Semantica**

- `err != nil` solo per vero errore DB o progetto inesistente
- `cfg == nil` significa `ntfy` non configurato per il progetto

Questo evita la falsa ambiguita' tra `sql.ErrNoRows` e `not configured`.

### 6. sendNtfy e logging diagnostico

**Comportamento richiesto**

`sendNtfy()` resta chiamata asincrona solo per nuovi issue e regressioni, ma:

- usa `GetProjectNtfyConfig(...)`
- se `cfg == nil`:
  - `slog.Debug("ntfy: not configured", "project", projectID)`
  - ritorna senza errore
- se errore DB:
  - `observe.Error(ctx, "ntfy: fetch config failed", err, "project", projectID)`
  - ritorna

**Logging**

- pre-send: `slog.Debug("ntfy: sending", "project", projectID, "config", cfg.Name, "url", cfg.URL, "topic", cfg.Topic)`
- post-send success: `slog.Info("ntfy: sent", "project", projectID, "status", statusCode)`
- post-send error HTTP:
  - `slog.Warn("ntfy: send failed", ...)`
  - includere `status`, `responseBody`
  - solo per `status >= 500` inviare anche Sentry best-effort

### 7. Admin API e UI per ntfy

**Nuovi endpoint admin**

- `GET /api/admin/ntfy-configs`
- `POST /api/admin/ntfy-configs`
- `PUT /api/admin/ntfy-configs/{id}`
- `DELETE /api/admin/ntfy-configs/{id}`
- `POST /api/admin/ntfy-configs/{id}/test`

**Payload configurazione**

```json
{
  "name": "Produzione",
  "url": "https://n.elmisi.com",
  "topic": "ampulla-errors",
  "token": "..."
}
```

**Project update API**

`PUT /api/admin/projects/{id}` cambia payload:

```json
{
  "name": "...",
  "slug": "...",
  "platform": "...",
  "ntfyConfigId": 12,
  "knownSdkVersion": "..."
}
```

Regole:

- `ntfyConfigId: null` => nessuna configurazione
- non si accettano piu' `ntfyUrl`, `ntfyTopic`, `ntfyToken`

**UI progetti**

Nel form progetto:

- rimuovere i tre campi inline `ntfy`
- aggiungere un `select`:
  - `Nessuna`
  - tutte le configurazioni `ntfy` disponibili
- mostrare un link a `#/ntfy`

**Nuova pagina admin `ntfy`**

- route: `#/ntfy`
- lista configurazioni con:
  - nome
  - url
  - topic
  - numero progetti collegati
- azioni:
  - crea
  - modifica
  - elimina
  - test

**Delete behavior**

- eliminare una configurazione `ntfy` non fallisce se e' usata
- i progetti collegati passano automaticamente a `ntfy_config_id = NULL` via `ON DELETE SET NULL`

### 8. Shutdown e hardening

**Contatori richiesti nel Processor**

Aggiungere:

- `activeWorkers atomic.Int64`
- `activeNtfy atomic.Int64`
- `queueDropCount atomic.Int64`

Uso:

- incrementare `activeWorkers` quando un worker inizia un job, decrementare a fine job
- incrementare `activeNtfy` quando parte la goroutine `ntfy`, decrementare alla fine

**Goroutine ntfy**

- tracciata nel `WaitGroup`
- wrappata con `defer observe.RecoverPanic(context.Background(), "ntfy", "project", projectID)`

**Worker**

- ogni job wrappato con `defer observe.RecoverPanic(context.Background(), "worker", "project", j.projectID)`

**Close()**

`Processor.Close()` deve:

1. fermare ticker
2. chiudere `done`
3. chiudere `queue`
4. attendere `wg.Wait()` in goroutine separata
5. fare timeout a `15s`

Se timeout:

- `slog.Error("processor shutdown timed out", "queue_len", len(p.queue), "active_workers", p.activeWorkers.Load(), "active_ntfy", p.activeNtfy.Load(), "timeout", "15s")`
- `sentry.CaptureMessage("processor shutdown timed out")` best-effort
- nessun panic

### 9. Request logging e cleanup loginAttempts

**Request logging**

Aggiungere middleware con:

- `method`
- `path`
- `status`
- `duration`
- `remote_ip`

Livelli:

- `Debug` per `< 400`
- `Info` per `>= 400`
- `/health` escluso

**loginAttempts**

- riusare il pattern di eviction lazy gia' usato in `internal/auth/middleware.go`
- non introdurre goroutine dedicate

### 10. Event retention

Resta opzionale.

- env: `AMPULLA_EVENT_RETENTION_DAYS`
- default `0`
- se `0`, nessuna retention sugli eventi
- se `> 0`, eliminare solo eventi di issue non `unresolved`

## Test Plan

### Ingest

- coda piena -> `Enqueue == false`
- `Envelope()` -> `503`
- `Store()` -> `503`
- `Retry-After` presente

### Observe

- `RecoverPanic` cattura panic e non rilancia
- stack trace presente nel log
- `Throttled` esegue una sola volta per intervallo

### ntfy service

- send success
- `401`
- `500`
- timeout
- config assente su progetto

### Migrazione ntfy

- progetti senza config -> `ntfy_config_id NULL`
- progetti con stesso `(url, topic, token)` -> una sola config
- progetti con stesso `url/topic` ma token diverso -> due config distinte

### Admin API / UI

- CRUD `ntfy-configs`
- test configurazione
- update progetto con `ntfyConfigId`
- delete config usata -> progetto passa a `none`

### Processor hardening

- panic in worker -> worker successivo continua
- panic in goroutine ntfy -> shutdown non si blocca
- timeout shutdown -> log con contesto completo

## Execution Order

1. introdurre `internal/observe`
2. introdurre backpressure `Enqueue -> bool` e `503` ingest
3. introdurre `ntfy_configurations` + migrazione `009`
4. introdurre `internal/notify/ntfy.go`
5. passare store e processor al nuovo modello `ntfy`
6. introdurre CRUD admin `ntfy-configs` e aggiornare form progetto
7. introdurre logging `ntfy` e test endpoint config
8. introdurre hardening worker/goroutine/shutdown
9. aggiungere request logging e cleanup `loginAttempts`
10. dopo verifica, creare migrazione `010` per rimuovere `projects.ntfy_*`

## Acceptance Criteria

- nessun handler ingest restituisce `200` se il job non entra in coda
- un progetto puo' usare una configurazione `ntfy` globale oppure nessuna
- il test `ntfy` avviene sulla configurazione globale reale usata dal sender
- panic HTTP, worker panic e ntfy panic sono loggati e catturati best-effort
- lo shutdown timed out lascia sempre un log locale con contesto sufficiente
- il piano puo' essere implementato senza ulteriori decisioni di prodotto o architettura
