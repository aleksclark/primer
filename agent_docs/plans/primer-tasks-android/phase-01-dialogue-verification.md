# Android Phase 1: Dialogue verification

## Goal

Complete and independently release the native `agent_dialogue` experience after
the main Tasks Phase 4 server/web contract is reviewed. A paired student opens a
reading occurrence, receives streamed questions and safe progress, answers three
questions, and sees the server-owned completion survive reconnect, process death,
and revocation boundaries.

## BDD Success Criteria

#### Scenario: Native three-question completion

- **Given** a paired student and a server-scheduled dialogue occurrence
- **When** the student gives two accepted answers, one insufficient answer plus
  follow-up, and a third accepted answer in the native app
- **Then** the UI streams questions/safe progress and shows exactly one completed
  occurrence from the durable server decision.

#### Scenario: Background and process restart resume

- **Given** two accepted questions
- **When** the app backgrounds, loses its socket, is force-stopped, and reopens
- **Then** it reloads the same attempt/history/count and only the final question
  remains
- **And** no message/evaluation is double-counted.

#### Scenario: Concurrent web/native turns recover safely

- **Given** the same student opens one attempt in browser and native app
- **When** both submit concurrently
- **Then** one turn is serialized/accepted and the other gets a typed conflict
  with resumable state
- **And** exactly one terminal decision can result.

#### Scenario: Revocation blocks further dialogue

- **Given** an active native attempt
- **When** the parent revokes/archives the bound student credential
- **Then** the next subscribe/message is denied, the app clears pairing and shows
  re-pair guidance, and durable parent audit remains.

#### Scenario: Student authority remains narrow

- **Given** prompt injection, answer-key request, foreign/cross-task IDs, or a
  request to edit/complete work
- **When** submitted natively
- **Then** the server exposes only scoped dialogue tools and no protected context,
  answer key, task mutation, or direct completion.

## Implementation Instructions

1. Consume the reviewed Phase 4 generated Kotlin REST/WS contract through one
   committed `StudentDialogueSocket` façade. Remove duplicate/raw socket or DTO
   paths.
2. Add the System C Decide/Learn screen: ruled transcript, current question,
   answer input/send, generic thinking/evaluating/retry/conflict/error/offline,
   accepted count, and completion summary. Camera/checklist navigation remains
   intact; no chat bubbles or raw reasoning.
3. Persist only non-secret dialogue cursor/occurrence/attempt metadata needed to
   restore UI. Bearer remains solely in the Keystore-encrypted token store.
4. Backgrounding closes delivery only; never call cancel implicitly. Reopen uses
   public state/replay routes and cursor. Explicit cancel is only shown where
   server policy allows it.
5. Add client message idempotency and expected sequence/version. Surface typed
   conflict/recovery without silently resending a semantically different answer.
6. Handle 401/403 revocation by clearing bearer/metadata/dialogue caches and
   returning to pairing; do not erase server evidence.
7. Add JVM/Compose tests for protocol parsing, reducers, transcript ordering,
   duplicate events, retry/conflict, no reasoning, background/reopen, and
   completion. Mock transport is supplemental to real emulator proof.

## End-to-End Test Plan

- Pair through the real parent web QR flow and real system Photo Picker fallback
  only if virtual-camera injection remains blocked.
- Start a real dialogue; answer two; background/force-stop/reopen; finish through
  insufficient/follow-up and third accepted answer.
- Submit concurrently from a real browser context and app; assert typed conflict
  and recovery with exactly one decision.
- Attempt injection/key/foreign/task-edit requests and inspect UI/wire/logs for no
  reasoning or protected content.
- Revoke mid-attempt from parent web and verify subscribe/message denial, local
  credential clearing, re-pair screen, and retained parent audit.
- After exploratory PASS, promote observed flows to connected instrumentation and
  black-box emulator automation; include process death and real WS/API.

Commands:

```bash
cd primer-tasks/android
./gradlew testDebugUnitTest assembleDebug connectedDebugAndroidTest
```

## Anti-Cheating Audit

- Trace app answer → generated façade → public student WS → durable message/job/
  evaluation/decision; reject local accepted counters sold as completion.
- Verify no token in DataStore, intent/query URI, transcript, screenshots, logs,
  or backup; only Keystore-encrypted custody is allowed.
- Confirm backgrounding does not cancel server run and reconnect loads DB state.
- Confirm tests do not inject accepted decisions, broad tools, system/rubric
  policy, or fake a paired app without public pairing.
- Search native wire/UI/storage evidence for raw reasoning and answer keys.

## Completion Gate

- [ ] All BDD scenarios pass on a fresh emulator against the real Tasks stack.
- [ ] Dedicated exploratory PASS precedes connected/black-box automation.
- [ ] JVM/Compose and promoted emulator suites are green.
- [ ] Browser/native concurrency and exactly-one decision evidence pass.
- [ ] Revocation and credential privacy checks pass.
- [ ] Fresh anti-cheat review approves the generated-client and server-authority boundaries.
