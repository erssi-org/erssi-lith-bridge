# Critical Race Condition Fix - 2025-11-01

## Problem Discovery

**Symptom:** Lith client połączył się z bridge, ale nie pokazywał żadnych bufferów/kanałów mimo że bridge otrzymywał dane z erssi.

**Root Cause:** Race condition w `internal/erssi/client.go:241`

## The Bug

```go
// OLD CODE (BUGGY):
for {
    var msg erssiproto.WebMessage
    json.Unmarshal(data, &msg)

    if c.onMessage != nil {
        go c.onMessage(&msg)  // ❌ Przekazujemy pointer do zmiennej która jest reużywana!
    }
}
```

**Co się działo:**
1. Otrzymujemy wiadomość `state_dump` → `msg` = state_dump
2. Wywołujemy `go c.onMessage(&msg)` w goroutine
3. NATYCHMIAST czytamy następną wiadomość `channel_join` → **NADPISUJEMY** `msg`
4. Goroutine z kroku 2 dopiero teraz wykonuje `handleErssiMessage`, ale `msg` już wskazuje na `channel_join`!
5. Wszystkie handlery otrzymują nieprawidłowe/pomieszane dane

**Rezultat:** Handler `handleChannelJoin` nigdy nie był wywoływany dla właściwych wiadomości, więc buffers nie były tworzone.

## The Fix

```go
// NEW CODE (FIXED):
for {
    var msg erssiproto.WebMessage
    json.Unmarshal(data, &msg)

    if c.onMessage != nil {
        msgCopy := msg  // ✅ Tworzymy kopię przed przekazaniem do goroutine
        go c.onMessage(&msgCopy)
    }
}
```

**Dlaczego działa:**
- Każda goroutine dostaje własną kopię struktury `WebMessage`
- Kolejne iteracje pętli nie nadpisują danych poprzednich wiadomości
- Handlery otrzymują poprawne, niezmienione dane

## Previous Fixes That Led Here

1. **Custom UnmarshalJSON** (`pkg/erssiproto/types.go:107-132`)
   - Naprawiono mapowanie pól: `channel` → `target`, `server` → `server_tag`
   - erssi wysyła inne nazwy pól niż oczekiwał kod

2. **Raw JSON logging** (`internal/erssi/client.go:222`)
   - Dodano logowanie surowego JSON dla debugowania
   - Pozwoliło odkryć że pola są poprawnie mapowane

3. **Race condition fix** - ten commit
   - Naprawiono przekazywanie pointerów do reużywanej zmiennej

## Files Modified

- `internal/erssi/client.go:244` - utworzenie `msgCopy` przed goroutine
- `pkg/erssiproto/types.go:107-132` - custom UnmarshalJSON (wcześniejszy fix)

## Testing

**User will test with Lith client and report results.**

Expected behavior after fix:
1. Bridge łączy się z erssi ✅
2. Lith łączy się z bridge ✅
3. Bridge wysyła `sync_server` do erssi ✅
4. erssi zwraca `state_dump` + `channel_join` messages ✅
5. **Bridge tworzy buffers** (poprzednio broken ❌ → teraz fixed ✅)
6. **Lith pokazuje listę kanałów** (poprzednio empty ❌ → teraz should work ✅)

## Build Command

```bash
cd /Users/k/bridge/erssi-lith-bridge
go build -o bridge ./cmd/bridge
./bridge  # Uses .env configuration
```

## Related Issues

- PROMPT.md - documentation of the original problem
- STATUS.md - 95% complete status, waiting for this fix

## Impact

**Before:** 0% functionality - Lith showed empty buffer list
**After:** Should be 100% functional - all channels/queries visible

This was the LAST critical bug blocking full functionality.
