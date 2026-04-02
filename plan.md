# Plan: Robustezza di Ampulla - no-loss guarantee + ntfy fix

## Context

Ampulla (v0.3.0) e' un error tracker Sentry-compatible scritto in Go. Il servizio funziona ma ha due classi di problema:

1. **Perdita di dati alla fonte:** `Processor.Enqueue()` scarta eventi silenziosamente quando la coda e' piena, mentre gli endpoint ingest rispondono comunque `200`. Questo e' il bug piu' grave: il client crede che l'errore sia stato registrato, ma non esiste nel DB.

2. **Cecita' operativa:** le notifiche ntfy non arrivano, e gli errori interni di Ampulla (panic, cleanup failure, queue drop) non vengono catturati dal self-monitoring Sentry. Questo non perde eventi, ma rende invisibili sia gli errori esterni (via ntfy) sia quelli interni (via Sentry).

> **NOTE**: Qui manca un chiarimento importante: il self-monitoring attuale copre solo il processo Go. Gli errori JavaScript dell'admin UI di Ampulla non vengono catturati dal `sentry-go` server-side. Se per "errori generati da ampulla stesso" includi anche il frontend admin, il piano deve prevedere un Browser SDK o almeno un hook `window.onerror` / `unhandledrejection`.

---

## Approach

Quattro fasi, ordinate per impatto sulla garanzia di non perdere errori:

1. **Ingest durability** — backpressure reale: `Enqueue` segnala il rifiuto, ingest risponde `503`, Sentry SDK riprova automaticamente
2. **Self-monitoring** — catturare errori interni di Ampulla in Sentry (panic, queue drop, cleanup failure, ntfy failure, serverError) oltre che in slog
3. **Fix ntfy** — logging diagnostico, helper condiviso, endpoint test, fix della causa specifica
4. **Hardening** — goroutine leak, panic recovery con stack trace, shutdown ordinato con drain, health check, request logging

Test automatici coprono i percorsi critici di ogni fase: queue full, DB failure, panic in worker/ntfy, errore HTTP ntfy, shutdown con backlog.

---

## Detailed Changes

### 1. Ingest durability / backpressure

**Problema:** `Enqueue()` in `internal/event/processor.go:75-81` usa un `select` con `default` che scarta silenziosamente l'evento e logga solo un warning. L'handler in `internal/api/ingest/handler.go:59` chiama `Enqueue` senza controllare il risultato e risponde sempre `200`.

Con Sentry SDK, una risposta `503` attiva il retry automatico con backoff. Rispondere `200` quando l'evento e' stato scartato e' il peggior scenario: il client non riprovera' mai.

**Modifiche in `internal/event/processor.go`:**

- `Enqueue` ritorna `bool` (accettato o no)
- Quando la coda e' piena: loggare a Warn **e** catturare una metrica/evento in Sentry (`sentry.CaptureMessage`)

```go
// Target shape
func (p *Processor) Enqueue(projectID int64, env *Envelope, sdkClient string) bool {
    select {
    case p.queue <- job{projectID: projectID, env: env, sdkClient: sdkClient}:
        return true
    default:
        slog.Warn("event queue full, dropping event", "project", projectID, "event", env.Header.EventID)
        sentry.CaptureMessage(fmt.Sprintf("event queue full: project=%d event=%s", projectID, env.Header.EventID))
        return false
    }
}
```

**Modifiche in `internal/api/ingest/handler.go`:**

- Sia `Envelope()` che `Store()` controllano il ritorno di `Enqueue`
- Se `false`, rispondere `503 Service Unavailable` con body JSON
- Sentry SDK riconoscera' il 503 e riprovera' con exponential backoff

```go
// Target shape (in Envelope handler, identico in Store)
if !h.processor.Enqueue(project.ID, env, sdkClient) {
    w.Header().Set("Retry-After", "60")
    http.Error(w, `{"error":"server overloaded"}`, http.StatusServiceUnavailable)
    return
}
```

**Test:** unit test con coda piena → verifica che `Enqueue` ritorni `false` e che l'handler risponda 503.

### 2. Self-monitoring: catturare errori interni in Sentry

**Problema:** oggi Ampulla ha il Sentry Go SDK configurato (`cmd/ampulla/main.go:37-49`) con tracing sui route admin/web, ma gli errori interni non vengono catturati. `slog` logga sul container stdout, ma senza dashboard centralizzata questi log si perdono.

> **NOTE**: C'e' anche un limite architetturale: Ampulla oggi si auto-invia gli errori a se stesso. In caso di DB down, queue full o shutdown degradato, proprio le catture Sentry proposte qui possono fallire. Per i failure mode critici il piano dovrebbe esplicitare che `slog` resta la fonte minima garantita, e valutare un progetto/istanza Sentry separata o almeno un fallback esterno per il self-monitoring.

**Errori da catturare:**

| Errore | File | Oggi | Dopo |
|--------|------|------|------|
| Queue drop | `event/processor.go:79` | slog.Warn | + sentry.CaptureMessage |
| Worker panic | `event/processor.go:67` | crash worker | recover + slog.Error + sentry.CurrentHub().Recover() |
| ntfy goroutine panic | `event/processor.go:239` | goroutine leak | recover + slog.Error + sentry.CurrentHub().Recover() |
| ntfy send failure | `event/processor.go:152-158` | slog.Warn | + sentry.CaptureMessage (solo HTTP >= 500, non 4xx config errors) |
| Cleanup failure | `event/processor.go:109` | slog.Error | + sentry.CaptureException |
| Admin serverError | `api/admin/handler.go:471` | slog.Error | + sentry.CaptureException |
| HTTP panic | `main.go:73` (Recoverer) | chi.Recoverer logga e 500 | aggiungere middleware Sentry pre-Recoverer |
| Startup/shutdown error | `main.go` | slog.Error + os.Exit | + sentry.CaptureException + sentry.Flush prima di exit |

**Implementazione:**

- Creare un helper `internal/observe/capture.go` con funzioni wrapper:
  - `observe.Error(err error)` → slog.Error + sentry.CaptureException
  - `observe.RecoverPanic(ctx)` → per defer, cattura panic con stack trace completo via `sentry.CurrentHub().RecoverWithContext(ctx, r)`
  - `observe.Message(msg string)` → slog.Warn + sentry.CaptureMessage
- Usare `runtime/debug.Stack()` nel recovery per loggare lo stack trace completo in slog
- Aggiungere middleware Sentry per panic HTTP in `main.go` (prima di `middleware.Recoverer`)

> **NOTE**: `observe.Message` non va usato in modo indiscriminato sui percorsi di overload. Se `queue full` genera una `CaptureMessage` per ogni evento rifiutato, rischi di amplificare il problema e di creare loop sul self-monitoring verso la stessa Ampulla. Qui serve rate limiting / deduplica temporale, oppure un semplice contatore/metrica locale invece di un evento Sentry per ogni drop.

**Test:** unit test che verifica che `observe.RecoverPanic` catturi correttamente un panic (mockando l'hub Sentry).

### 3. Fix notifiche ntfy

**Chiarimento importante:** il bug ntfy non fa sparire eventi. `sendNtfy()` viene chiamata solo dopo `InsertEvent()` ha gia' scritto nel DB. Qui si perde la **notifica operativa**, non l'errore.

**Problema principale:** `sendNtfy()` in `internal/event/processor.go:117-124` esce silenziosamente in due casi distinti senza alcun log:
- Errore DB nella query `GetProjectNtfyConfig` → swallowed
- Config ntfy assente (colonne NULL) → swallowed

**Problema secondario:** l'admin API in `internal/store/postgres.go:417-438` non distingue tra "campo lasciato invariato" e "campo svuotato" (entrambi → NULL). La semantica funziona ma non e' documentata e puo' confondere.

**Modifiche in `internal/event/processor.go` — logging:**

- Separare i due casi di uscita anticipata con log distinti:

```go
// Target shape
if err != nil {
    slog.Warn("ntfy: db error fetching config", "project", projectID, "error", err)
    return
}
if ntfyURL == "" || ntfyTopic == "" {
    slog.Debug("ntfy: not configured", "project", projectID)
    return
}
```

- Aggiungere log Debug pre-invio: `"ntfy: sending"` con endpoint (senza token)
- Aggiungere log Info post-invio: `"ntfy: sent"` con status code
- In caso di errore HTTP, leggere e loggare anche il response body (ntfy restituisce messaggi utili)
- Per errori HTTP >= 500 dal server ntfy: catturare anche in Sentry (via `observe.Message`)

**Estrarre helper condiviso per l'invio:**

- Creare un metodo `doSendNtfy(ctx, projectName, ntfyURL, ntfyTopic, ntfyToken, title, body, clickURL) (statusCode int, responseBody string, err error)` che:
  - Costruisce e invia la richiesta HTTP
  - Ritorna status, body di risposta, ed errore normalizzato
  - Usato sia da `sendNtfy` (nel worker) sia dal nuovo endpoint test

**Endpoint test-ntfy in `internal/api/admin/handler.go`:**

- `POST /api/admin/projects/{id}/test-ntfy`
- Recupera config dal DB, chiama `doSendNtfy` con messaggio di test
- Ritorna JSON con `{status, body, error}` — il risultato reale, non un successo finto
- Condivide esattamente la stessa logica di invio di `sendNtfy`

**Bottone UI in `internal/admin/static/pages/projects.js`:**

- Bottone "Test ntfy" nel form di modifica progetto
- Mostra il risultato inline (successo verde, errore rosso con dettagli)

**Test:** unit test con mock HTTP server: successo, 401, 500, timeout, config mancante.

### 4. Goroutine leak e panic recovery in sendNtfy

**Problema:** goroutine fire-and-forget non tracciate dal WaitGroup, senza recovery, che possono sopravvivere a `processor.Close()`.

**Fix in `internal/event/processor.go`:**

```go
// Target shape
if result.IsNew || result.IsRegression {
    p.wg.Add(1)
    go func() {
        defer p.wg.Done()
        defer observe.RecoverPanic(context.Background())
        p.sendNtfy(projectID, result)
    }()
}
```

`observe.RecoverPanic` cattura il panic con stack trace completo (`runtime/debug.Stack()`), logga in slog a Error, e invia a Sentry.

### 5. Panic recovery nei worker

**Fix in `internal/event/processor.go`:**

```go
// Target shape
func (p *Processor) worker() {
    defer p.wg.Done()
    for j := range p.queue {
        func() {
            defer observe.RecoverPanic(context.Background())
            p.Process(context.Background(), j.projectID, j.env, j.sdkClient)
        }()
    }
}
```

Stesso pattern: recovery con stack trace + Sentry. Il worker sopravvive e continua a processare la coda.

**Test:** unit test che inietta un job che causa panic → verifica che il worker sopravviva e processi il job successivo.

### 6. HTTP client dedicato per ntfy

**Motivazione corretta:** il `context.WithTimeout(10s)` gia' limita la durata della chiamata HTTP. Un client dedicato serve per configurare il `Transport` (connection pooling, keep-alive, TLS handshake timeout) e per non inquinare `http.DefaultClient` usato da altri componenti (es. Sentry SDK).

**Fix:** creare `ntfyClient *http.Client` nel Processor con Transport esplicito.

### 7. Graceful shutdown con drain

**Problema:** lo shutdown attuale non ha una fase esplicita di "smetti di accettare, drena la coda". Durante un rollout, eventi possono arrivare mentre il processor sta chiudendo.

**Sequenza corretta:**

1. Signal ricevuto
2. `srv.Shutdown(10s)` — smette di accettare nuove connessioni HTTP, drena in-flight
3. `processor.Close()` — chiude la coda (nessun nuovo job), aspetta che i worker finiscano i job rimanenti + goroutine ntfy
4. Se i worker non finiscono entro 15s: log Error, `sentry.CaptureMessage("processor shutdown timed out")`, `sentry.Flush(2s)`, exit con codice non-zero
5. `db.Close()` — chiude il pool

Il timeout al punto 4 e' un **failure mode** da evidenziare e ridurre al minimo, non un comportamento "accettabile". Va loggato, catturato in Sentry, e idealmente non dovrebbe mai scattare con il dimensionamento attuale (1k-5k eventi/mese, coda di 1000).

> **NOTE**: Anche qui vale lo stesso caveat: se `SENTRY_DSN` punta alla stessa istanza, durante shutdown degradato la `CaptureMessage` finale potrebbe non arrivare. Conviene trattarla come best-effort e assicurarsi che il log locale contenga sempre abbastanza contesto (`queue_len`, job in corso, timeout esatto).

**Fix in `internal/event/processor.go`:**

```go
// Target shape
func (p *Processor) Close() {
    p.ticker.Stop()
    close(p.done)
    close(p.queue)

    done := make(chan struct{})
    go func() {
        p.wg.Wait()
        close(done)
    }()

    select {
    case <-done:
        slog.Info("processor shutdown complete")
    case <-time.After(15 * time.Second):
        slog.Error("processor shutdown timed out, events in queue may be lost")
        sentry.CaptureMessage("processor shutdown timed out")
        sentry.Flush(2 * time.Second)
    }
}
```

### 8. Health check container

Utile operativamente ma non critico per no-loss. Priorita' inferiore rispetto ai punti 1-7.

**Fix in `docker-compose.yml`:** aggiungere healthcheck con `wget --spider http://localhost:8090/health`.

### 9. Request logging + middleware Sentry per panic HTTP

**Due componenti:**

1. **Request logging middleware** in `cmd/ampulla/main.go`: metodo, path, status, durata, IP. Info per >= 400, Debug per successi. Escludi `/health`.

2. **Middleware Sentry per panic HTTP**: registrato **prima** di `middleware.Recoverer`, cattura i panic con `sentry.CurrentHub().RecoverWithContext()` prima che Recoverer li trasformi in 500 senza traccia.

> **NOTE**: Il middleware di panic dovrebbe essere globale, non limitato alle route admin/web dove oggi c'e' il tracing. Se un panic avviene sugli endpoint ingest, e' proprio uno degli errori piu' importanti da auto-catturare.

### 10. Pulizia mappa loginAttempts

Hardening utile ma non sul critical path. Priorita' bassa.

**Fix in `internal/admin/auth.go`:** aggiungere eviction periodica di tutta la mappa dentro `allowLogin` (pattern gia' usato in `internal/auth/middleware.go:evictExpired`).

### 11. Event retention cleanup (opt-in, default disabilitato)

**Nota:** questo punto e' in tensione con il requisito di non perdere errori. Lo rendiamo esplicitamente opt-in.

- Nuovo env var `AMPULLA_EVENT_RETENTION_DAYS` (default: `0` = conservazione indefinita)
- Se > 0: la cleanup goroutine cancella events piu' vecchi di N giorni, mantenendo le issues (che hanno `first_seen`, `last_seen`, `event_count`)
- La cancellazione non tocca events di issue con status `unresolved`

---

## Edge Cases and Risks

- **503 e retry SDK:** Sentry SDK riprova automaticamente su 429/503. Se la coda resta piena a lungo, il client accumulera' eventi localmente e li inviera' quando Ampulla torna disponibile. Il `Retry-After: 60` header da' al client un hint ragionevole.
- **test-ntfy endpoint:** espone l'invio di notifiche, ma e' protetto da session auth admin. Rischio basso.
- **Panic recovery:** un panic indica un bug serio. Il recovery evita di perdere un worker, il bug viene catturato in Sentry con stack trace completo per permettere investigazione.
- **Shutdown timeout:** e' un failure mode, non un comportamento accettato. Se scatta: log Error, cattura Sentry, e indica un problema di dimensionamento o di I/O bloccante da investigare.
- **Event retention:** disabilitato di default. Attivandolo, l'utente accetta esplicitamente la cancellazione.

---

## Open Questions

1. **Le colonne ntfy nel DB sono effettivamente popolate?** Verificare: `SELECT id, name, ntfy_url, ntfy_topic FROM projects;`. Se tutte NULL, il problema e' che la config non e' stata salvata.

2. **L'ntfy server (`n.elmisi.com`) richiede autenticazione?** Testare con curl diretto.

3. **Che comportamento vuoi quando la coda e' piena?** Il piano propone `503` con `Retry-After` (che Sentry SDK gestisce nativamente). Alternative: `429` (semantica diversa), blocking con timeout (rischia di bloccare le goroutine HTTP).

4. **Quali errori interni di Ampulla vuoi assolutamente catturare in Sentry?** Il piano propone: panic HTTP, panic worker, panic ntfy, queue drop, cleanup failure, serverError admin, ntfy server error (>= 500), startup/shutdown error. Troppo? Troppo poco?

> **NOTE**: A questa domanda aggiungerei esplicitamente: vuoi coprire anche gli errori del frontend admin di Ampulla? Se si', serve decidere se introdurre un progetto Sentry separato per `ampulla-admin-ui`, per non mischiare errori browser e backend.

5. **Il `SESSION_SECRET` e' configurato nel `.env` o viene auto-generato?** Se auto-generato, ogni restart invalida le sessioni admin.

6. **Vuoi attivare l'event retention?** Se si', quale periodo?

---

## Task Breakdown

### Fase 1 — Ingest durability
- [ ] T1: `Enqueue` ritorna `bool`, ingest handler risponde `503` su rifiuto
- [ ] T2: Test automatico: coda piena → `Enqueue` false → handler 503

### Fase 2 — Self-monitoring
- [ ] T3: Creare `internal/observe/capture.go` con `Error()`, `RecoverPanic()`, `Message()`
- [ ] T4: Integrare `observe` in: queue drop, worker panic, ntfy panic, cleanup failure, serverError admin
- [ ] T5: Aggiungere middleware Sentry per panic HTTP (prima di Recoverer)
- [ ] T6: Catturare errori startup/shutdown in Sentry prima di exit

> **NOTE**: Se includi l'admin UI nel perimetro di "errori di Ampulla", qui manca un task dedicato per il frontend: Browser SDK o bridge minimale `window.onerror`/`unhandledrejection` verso un endpoint server-side. Senza questo la fase "Self-monitoring" resta backend-only.

### Fase 3 — Fix ntfy
- [ ] T7: Logging diagnostico completo in `sendNtfy()` (separare errore DB da config assente)
- [ ] T8: Estrarre helper `doSendNtfy()` condiviso tra worker e test endpoint
- [ ] T9: Endpoint `POST /api/admin/projects/{id}/test-ntfy` + bottone UI
- [ ] T10: Verificare config ntfy nel DB e testare con curl diretto
- [ ] T11: Test automatici: mock HTTP server (successo, 401, 500, timeout, config mancante)

### Fase 4 — Hardening
- [ ] T12: Fix goroutine leak sendNtfy (WaitGroup + observe.RecoverPanic)
- [ ] T13: Panic recovery nei worker (observe.RecoverPanic + test)
- [ ] T14: HTTP client dedicato per ntfy con Transport esplicito
- [ ] T15: Graceful shutdown con drain + timeout come failure mode (Sentry capture)
- [ ] T16: Health check container in docker-compose
- [ ] T17: Request logging middleware
- [ ] T18: Fix memory leak loginAttempts
- [ ] T19: (Opt-in) Event retention cleanup, default disabilitato
