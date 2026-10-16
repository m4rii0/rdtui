# Application lifecycle and boundaries

This describes the implemented contracts, not a persistent job scheduler. Queues
and pending credentials exist only for the lifetime of one process.

## Composition and dependency direction

`cmd/rdtui/main.go` owns signals, process inputs, concrete adapters, Bubble Tea,
and cleanup. `productionOptions` captures environment overrides and supplies the
HTTP client, clock, cancelable wait, file inspection, desktop adapter, and backend
factory to `app.NewApplication`. `applicationDependencies` exposes only the
feature contracts to the TUI.

```text
cmd/rdtui
  ├─ config.Store → config.Repository → EffectiveConfig
  ├─ app.Application
  │    ├─ Runtime
  │    ├─ Authentication → auth.Manager → OAuth client / credential store / preferences
  │    ├─ TorrentService → authenticated realdebrid.Client → auth.Manager.Token
  │    ├─ BatchService → BatchAPI / Desktop / WaitFunc
  │    ├─ DownloadCoordinator → download.Backend (directdl.Manager or aria2.Manager)
  │    └─ BulkQueue → Runtime / DownloadCoordinator / PrepareDownload / WaitFunc
  └─ tui.Model ← tui.Dependencies
```

`internal/app`, `internal/auth`, and `internal/config` do not depend on the TUI,
Bubble Tea, or Lip Gloss. Application consumers own small adapter interfaces such
as `TorrentAPI`, `BatchAPI`, `Desktop`, and `auth.CredentialStore`. The TUI owns
`AuthService`, `TorrentService`, `DownloadService`, `BatchService`,
`BulkQueueService`, and `DesktopService` at their feature boundaries. There is no
second application facade owning a client, session, or active download.

The root model routes messages and owns navigation, size, and overlays;
`authModel`, `torrentsModel`, and `downloadsModel` hold feature state. Presentation
owns selection, cursor, filtering, confirmations, and help. Application workflows
own remote execution, queue transitions, result accounting, and filename/existing
file checks. `internal/demo.Service` satisfies the same consuming interfaces;
demo batches and queues use the real application workflows with simulated I/O.

## Contexts, admission, and shutdown

There are two cancellation levels:

1. **Caller/request lifetime.** `Runtime.Do` and `Runtime.Spawn` register work
   before releasing the admission lock. Their operation context follows both the
   caller and runtime. Feature `asyncRequest` generations cancel superseded
   detail/auth/setup requests and reject late results. Canceling a start waiter
   ends that caller's wait; it does not undo an admitted transfer.
2. **Application/transfer lifetime.** `DownloadCoordinator` creates the backend's
   transfer context from its own application parent. `BulkQueue.Start` uses its
   caller only for admission, then owns a runtime worker. Leaving a download
   screen, including `esc` during bulk execution, keeps that work running.
   Internal queue cancellation cancels the owned transfer, not just observation.
   No new user-facing cancel shortcut is provided.

The entry point uses `signal.NotifyContext` for interrupt and SIGTERM. Quit stops
runtime admission; `cleanupRuntime` supplies a fresh background-derived **2-second
deadline**, shared by application/backend closure and runtime draining.
`Application.Close` stops its runtime before closing downloads and waiting for
workers. Errors are joined and reported, including an expired cleanup deadline.
Repeated closes wait on the same eventual completion; a timed-out caller does not
reopen admission or abandon cleanup. Do not use the canceled root context as the
cleanup context.

**Observed aria2 limitation:** the coordinator currently starts backend shutdown
with a background context. Aria2's internal process-exit grace period is five
seconds, whereas the CLI waits only two seconds. A local real-aria2 smoke run
therefore reported `context deadline exceeded` on quit even after a successful
download. The caller's wait is bounded, but clean aria2 exit within that budget
is not established by the passing fake-backend tests.

Both backends implement only `StartDownload(ctx, request)`,
`DownloadStatus(ctx, id)`, and `Shutdown(ctx)` through `download.Backend`.
Admission and worker registration share a lock with shutdown. Late starts match
`download.ErrClosed`; canceled starts are rejected before admission. No metadata
lock is held over network, file, RPC, or process waits. Direct HTTP follows the
transfer context. Aria2 has a registered cancellation worker that removes the
owned GID and confirms termination; backend shutdown interrupts RPC waits and
stops the owned process. Binary configuration is supplied to `aria2.NewManager`.

## Download identity and queue leases

`DownloadCoordinator` alone owns the current `DownloadSnapshot`. A `DownloadRef`
contains a monotonic generation plus backend GID. Concurrent starts share one
reservation; a further single-download request reopens the active transfer rather
than starting another. Backend calls happen outside the metadata lock and results
commit only to the matching reference. At most one status query is in flight for
that reference; a caller canceling its wait does not detach or duplicate the query.

A transient status error is separate from a confirmed terminal backend failure:
retain the last snapshot and retry, without freeing admission. `ErrBackendLost`
can establish terminal failure. TUI results carry feature generation and download
reference; queue observations also check queue ID and item identity. Old and
duplicate terminal messages cannot advance the next item.

`BulkQueue` reserves a `QueueLease` for the **entire queue**, including URL
resolution, overwrite prompts, and gaps. An unrelated active download rejects
queue admission with `ErrActiveDownload`. A second queue receives `ErrQueueActive`.
A single start during a lease reopens the queued transfer if one exists; otherwise
it receives `ErrQueueActive` and returns to queue presentation. It cannot insert
work into the queue. Only the queue's admitted reference can finish its item.

`QueueSnapshot` is an immutable observation (nested mutable results are copied).
UI polls do not drive execution. `QueueDecision` identifies queue ID and item
index; obsolete/repeated decisions match `ErrStaleQueue`. Declining overwrite
skips the item and advances. Cancellation unblocks an overwrite wait and records
unstarted items as canceled. Internal queue cancellation retains its lease until
the transfer is confirmed stopped, using an independent **2-second stop budget**
shortened by an earlier runtime root deadline. The `Close` caller independently
bounds its drain wait.

### Ambiguous starts and stops require restart

If aria2 returns a GID but the first status lookup fails, it remains an admitted
waiting transfer and polling recovers. An explicit rejected start frees its
reservation. A transport failure after `AddURI` without a known GID matches
`download.ErrUncertainStart`: downloader admission closes, the owned backend is
shut down, the current queue item fails, and unstarted items are canceled. A stop
that cannot be confirmed matches `ErrTransferStopUnconfirmed` and likewise faults
admission. **Restart the application before more downloads.** Neither case is
automatically retried; doing so could duplicate a transfer.

## Batches, outcomes, and timing

`BatchService` executes delete, link resolution/copy, file-selection submission,
and local import in confirmed order; asynchronous plans clone nested ID slices.
Ordinary item failures continue. Each planned unit has one success, failed,
skipped, or canceled outcome. In-flight canceled mutations can be
`RemoteOutcomeUnknown`; completed results survive cancellation and unstarted work
is canceled. There is no automatic replay of ambiguous mutations.

Link outcomes retain torrent ID, target index, URL, and error. Empty URLs fail.
`ResolvedURLs` retains successful targets in order, including duplicates;
`CopiedURLs` counts only URLs passed to a successful desktop clipboard write and
is zero when unavailable or failed. Clipboard failure does not discard resolved
URLs. Torrent aggregates derive from target outcomes: all success is success,
some success is partial, no success with failure is failed, all skipped is skipped,
otherwise canceled. Cleanup preselects only fully successful torrents; partial,
failed, skipped, and canceled torrents remain unselected unless explicitly toggled.

| Work | Timing contract |
|---|---|
| Download and bulk observation | 1 second after completion of a poll; no overlapping poll |
| Torrent-list refresh | 5 seconds, with in-flight protection |
| Batch delete, copy, file-selection submissions | Cancelable 250-millisecond inter-request delay |
| Device token refresh | Missing token or expiry within 1 minute (inclusive) |
| Shared credential refresh | Application lifetime, capped at 30 seconds |
| Default exit cleanup / internal queue stop | 2 seconds, not a new budget per shutdown stage |

Clock, delay, HTTP transport, backend, filesystem inspection, credential storage,
and desktop operations have injection seams. Tests synchronize with barriers and
controlled waits rather than relying on sleeps for concurrency assertions.

## Session ownership and persistence failures

`auth.Manager` is authoritative. The TUI sees display snapshots, not reusable
bearer tokens. `realdebrid.Client` asks the injected `Token` source at each
authenticated request; the separate OAuth client does not invoke that source.
Precedence is nonempty environment private token, persisted private token, stored
device credentials, then interactive auth. An invalid explicit token is reported
without silently falling back to OAuth. Private tokens never refresh.

Concurrent device refreshes coalesce per session generation. Each caller can
cancel its wait independently while the runtime-owned refresh remains bounded.
Interactive replacement changes generation and serializes durable commit with
replacement; an old refresh cannot overwrite a new session or credentials.

Freshly issued device credentials must be saved **before** they become usable for
API requests. On save failure, `CredentialPersistenceError` retains the cause and
the manager keeps the new credentials, including any rotated refresh token, in
memory as pending. Retry persistence before any further OAuth exchange/refresh;
after a successful save, refresh normally if the retained token has expired.
This applies to both refresh and interactive device auth. Intentional session
replacement discards the old generation's pending state. Pending state is not
durable: after exit reauthentication may be needed. Do not silently reuse the old
refresh token or treat an unsaved session as authenticated.

Missing `auth.json` means no stored credentials. Malformed JSON, permissions, and
other I/O errors remain distinguishable through `CredentialLoadError`.
Unauthenticated operations match `auth.ErrNoValidCredentials`. Structured
`realdebrid.APIError` preserves HTTP status and API code (including rate limits).
Confirmed credential rejection is attributed to the exact token sent: a late
old-token rejection cannot invalidate a newer session. Mutations are not replayed
after rejection or ambiguous transport errors.

## Configuration provenance and atomic replacement

`config.Config` is the persisted preference DTO. `config.Repository` owns it and
serializes narrow updates; `EffectiveConfig` separately combines preferences,
explicit `Overrides`, and defaults. Unrelated saves never persist environment
tokens/endpoints or computed defaults. Explicit token persistence saves the
entered token, even when the environment overrides its effective value.

Empty or invalid backend resolves to **direct** everywhere; aria2 is explicit.
The default directory is `<home>/Downloads`; missing required home/config paths
are reported rather than silently inventing a directory. Effective construction
configuration is captured at startup; edit backend/path settings before restarting.

`config.Store` writes `config.json` and `auth.json` with existing `encoding/json`
field names and omission behavior, without migration. Replacement writes a sibling
temporary file with restrictive permissions, writes/syncs/closes it, then atomically
replaces the destination. POSIX uses same-directory rename; Windows uses
`MoveFileEx` with replace-existing semantics. Failures before replacement preserve
old bytes and clean temporary files. The repository publishes new in-memory
preferences only after replacement succeeds. This is in-process serialization,
not a cross-process lock, cross-file transaction, or power-loss durability promise.

## Acceptance traceability

Representative executable evidence below is in addition to the existing sorting,
slice ownership, and JSON regression suite. Test names are searchable with
`go test ./... -run '<name>'`; the full gates remain authoritative.

| Requirement | Evidence (package: test names) |
|---|---|
| R1 runtime/cancellation | app: `TestRuntimeRejectsWorkAfterStop`, `TestRuntimeCancelsAndDrains`, `TestRuntimeWaitDeadline`, `TestApplicationCloseDuringRefresh`, `TestApplicationCloseDuringOverwrite`; tui: `TestQuitCancelsOutstandingCommand`, `TestBulkApplicationRunsWithoutPresentation` |
| R2 backend lifecycle | directdl and aria2: `TestStartRacingWithShutdown`, `TestConcurrentShutdown`, `TestShutdownDeadlineCanBeRetried`; download: `TestStartAfterShutdown`, `TestCanceledStartNotAdmitted`; directdl: `TestTransferContextCancelsHTTP`; aria2: `TestTransferContextCancelsOwnedGID`, `TestRPCAdapterCloseInterruptsBlockedWrite` |
| R3 identity/single transfer | app: `TestConcurrentStartsShareReservation`, `TestConcurrentStatusQueriesCoalesce`, `TestStatusFailureDoesNotReleaseActiveSlot`, `TestLateStatusCannotOverwriteCurrent`, `TestUncertainStartRequiresRestart`, `TestQueueDuplicateCompletionAdvancesOnce`; tui: `TestLateDownloadResultIgnored`, `TestDuplicateTerminalResultIgnored`; aria2: `TestAdmittedGIDSurvivesStatusError` |
| R4 batch results | app: `TestBatchPreservesOrderAndContinues`, `TestBatchCancelStopsRequests`, `TestCanceledMutationOutcomeUnknown`, `TestCopyCountsTargetsNotTorrents`, `TestClipboardFailureRetainsResolvedURLs`, `TestSelectionCopiesNestedIDs`; tui: `TestBatchCopyRendersExactUnits` |
| R5 queue/TUI | app: `TestQueueAdvancesWithoutUI`, `TestQueueLeasePreventsInterleavedStarts`, `TestQueueStaleDecisionRejected`, `TestQueueCancelWaitsForTransferStop`, `TestQueueStopUnconfirmedRequiresRestart`; tui: `TestBulkApplicationRunsWithoutPresentation`, `TestBulkOverwriteDecisionUsesItemIdentity`, `TestBulkCleanupUsesTerminalOutcomes`, `TestBulkCleanupCanceledAndSkippedNotPreselected` |
| R6 injection | app: `TestCompositionUsesInjectedDependencies`, `TestAuthenticatedOperationsBeforeLoginReturnAuthRequired`; demo: `TestDemoSatisfiesFeatureContracts`, `TestDemoBulkQueueUsesSharedAdmissionAndRunsWithoutUI`; tui: `TestDownloadFeatureNeedsOnlyDownloadDependencies` |
| R7 session | auth: `TestConcurrentRefreshCoalesced`, `TestRefreshWaiterCanCancel`, `TestRefreshBoundedByRootAndTimeout`, `TestPendingRotatedCredentialsRetrySave`, `TestInteractiveDeviceCredentialsPendingSave`, `TestOldRefreshCannotSaveOverNewDeviceSession`, `TestMissingCredentialsDifferFromCorruptOrUnreadable`; app: `TestSessionRefreshReachesTorrentRequests`, `TestApplicationCloseDuringRefresh`; realdebrid: `TestAPIErrorPreservesStatusAndCode`, `TestMutationNotAutomaticallyReplayed` |
| R8 configuration | config: `TestAtomicSavePreservesOldFileOnFailure`, `TestAtomicSaveCleansTemporaryFiles`, `TestAtomicSaveReplacesExistingFile`, `TestUnrelatedSaveDoesNotPersistEnvironment`, `TestExplicitTokenSaveUsesEnteredToken`, `TestConcurrentPreferenceUpdatesPreserveBothChanges`, `TestFailedSaveDoesNotPublishPreferences`, `TestResolvePreferences`, `TestStoredJSONCompatibility` |
| R9 verification | `make verify` (format, tidy diff, vet/lint, full tests, build), `make test-race`, `make build-all`; `.github/workflows/verify.yml` runs Linux verification/race and native Linux/macOS/Windows persistence tests |

Cross-compilation is not native execution. Native persistence CI must pass on the
target runners; fixture OAuth/RPC tests do not establish live-service behavior.
