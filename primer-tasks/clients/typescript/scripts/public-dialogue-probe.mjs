// Conformance bootstrap only: Node lacks browser cookie custody/Origin. These
// temporary credentials arrive over stdin, stay in memory, and are never logged.
// All product operations still use the real generated client facade.
import { loadTestClient } from "./client-test-support.mjs";
let input = "";
for await (const chunk of process.stdin) { input += chunk; if (input.length > 16384) throw new Error("Probe input too large"); }
const context = JSON.parse(input);
const origin = new URL(context.baseUrl).origin;
if (!/^http:\/\/127\.0\.0\.1:\d+$/.test(origin)) throw new Error("Owned loopback fixture required");
globalThis.window = { location: new URL(origin) };
globalThis.document = { cookie: `tasks_csrf=${context.csrf}` };
const nativeFetch = globalThis.fetch, NativeSocket = globalThis.WebSocket;
let restSchemaLink = false;
const transport = async request => {
  const url = new URL(request.url);
  if (url.origin !== origin) throw new Error("Foreign conformance request");
  request.headers.set("Cookie", url.pathname.startsWith("/student/") ? context.studentCookies : context.parentCookies);
  request.headers.set("Origin", origin);
  const response = await nativeFetch(request);
  if (url.pathname.endsWith("/dialogue") && response.ok) { const body = await response.clone().json(); restSchemaLink ||= typeof body.$schema === "string"; }
  return response;
};
const loaded = await loadTestClient();
let dialogue;
try {
  const api = loaded.client.createTasksClient({ baseUrl: context.baseUrl, fetch: transport });
  const state = await api.startStudentDialogue(context.occurrenceId);
  dialogue = loaded.client.createDialogueClient({ occurrenceId: state.occurrenceId, attemptId: state.attemptId, baseUrl: context.baseUrl, reconnect: false,
    webSocketFactory: (url, protocols) => new NativeSocket(url, { protocols, headers: { Origin: origin, Cookie: context.studentCookies } }),
  });
  const waitFor = predicate => new Promise((resolve, reject) => {
    let unsubscribe, settled = false;
    const timer = setTimeout(() => { settled = true; unsubscribe?.(); reject(new Error("Generated client dialogue deadline")); }, 10000);
    unsubscribe = dialogue.subscribe(snapshot => {
      if (settled) return;
      if (snapshot.error && !snapshot.error.retryable) { settled = true; clearTimeout(timer); unsubscribe?.(); reject(new Error("Generated client protocol failure")); }
      else if (predicate(snapshot)) { settled = true; clearTimeout(timer); unsubscribe?.(); resolve(snapshot); }
    });
    if (settled) unsubscribe();
  });
  dialogue.connect();
  await waitFor(snapshot => snapshot.canAnswer);
  dialogue.sendMessage("I do not know.");
  await waitFor(snapshot => snapshot.canAnswer && snapshot.events.some(event => event.kind === "answer_evaluation" && event.status === "rejected"));
  const answers = ["The family repaired the garden wall after the storm.", "The mortar must dry before the next course of stones.", "Rushing the work would weaken the wall."];
  for (let i = 0; i < answers.length; i++) {
    dialogue.sendMessage(answers[i]);
    await waitFor(snapshot => i === 2 ? snapshot.completed : snapshot.canAnswer && snapshot.binding?.acceptedCount === i + 1);
    if (i === 1) { dialogue.disconnect(); dialogue.connect(); await waitFor(snapshot => snapshot.canAnswer && snapshot.binding?.acceptedCount === 2); }
  }
  const inspection = await api.inspectOccurrence(context.occurrenceId, { limit: 50 });
  const evaluations = inspection.entries?.filter(entry => entry.kind === "answer_evaluation") ?? [];
  if (inspection.acceptedCount !== 3 || evaluations.length !== 4 || inspection.status !== "completed") throw new Error("Generated inspect result does not match completion");
  let typedError = false;
  try { await api.inspectOccurrence("00000000-0000-0000-0000-000000000000"); } catch (error) { typedError = error instanceof loaded.client.TasksApiError && error.status === 404; }
  if (!typedError) throw new Error("Generated typed error was not preserved");
  console.log(JSON.stringify({ completed: true, accepted: 3, evaluations: evaluations.length, typedError, restSchemaLink }));
} catch (error) {
  // Only safe error classification, not raw response/provider/socket buffers.
  console.error(JSON.stringify({ error: "generated-client-conformance-failed", type: error?.name ?? "Error", status: error?.status ?? null }));
  process.exitCode = 1;
} finally { dialogue?.disconnect(); await loaded.cleanup(); }
