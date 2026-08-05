# Lesson 13: Pipes and redirection

## Objectives

By the end of this lesson, the student can:

- define **standard input**, **standard output**, and **standard error** as three distinct streams;
- identify stdin, stdout, and stderr by file descriptor numbers 0, 1, and 2;
- trace data from one command's stdout through `|` to another command's stdin;
- redirect stdout to a file with `>`;
- explain that `>` replaces an existing destination and therefore presents overwrite risk;
- append stdout to an existing file with `>>` without removing its current content;
- redirect stderr separately with `2>` while leaving stdout on the terminal;
- predict whether output will appear on screen or in a file;
- inspect exact final file content after redirection; and
- preserve supplied source fixtures while changing only named disposable destinations.

## Standards

These references use the current custom standards while more specific command-line stream and redirection standards are pending.

- **Primary — `PRIMER.DL.6.FILES.2`:** Inspect file contents with safe tools and confirm expected text. The student selects records, follows output streams, and verifies exact redirected file content while preserving source material.
- **Reinforcement — `PRIMER.DL.6.FILES.1`:** Create and organize files according to an explicit structure. The student deliberately replaces and then extends a safe report file and creates a separate diagnostic record.

The activity records `standardsStatus: pending-new-standards` in metadata so this provisional alignment is explicit.

## The three standard streams

A command starts with three standard communication channels, called **streams**:

| Descriptor | Name | Abbreviation | Ordinary role |
|---:|---|---|---|
| 0 | standard input | stdin | data the command reads |
| 1 | standard output | stdout | ordinary result the command writes |
| 2 | standard error | stderr | diagnostics and error messages the command writes |

The descriptor numbers matter because shell redirection syntax can name a stream. In `2> errors.txt`, the `2` means stderr.

When no redirection is present in an interactive terminal:

- stdin usually comes from the keyboard;
- stdout usually appears on the terminal; and
- stderr usually also appears on the terminal.

Because stdout and stderr can both appear in the same terminal, they may look like one kind of output. They remain separate streams. The shell can route each one to a different place.

A useful mental model is plumbing:

```text
stdin (0)  --->  COMMAND  --->  stdout (1)
                     \
                      `------->  stderr (2)
```

A command can use all three in one run. It can read input, produce an ordinary result, and also report a diagnostic.

## Shell operators are routing instructions

The shell interprets `|`, `>`, `>>`, and `2>` before launching the relevant programs. They are not ordinary arguments to `grep`, `wc`, or `check-sensor`.

| Operator | Route |
|---|---|
| `|` | left command's stdout to right command's stdin |
| `>` | stdout to a file, replacing that file first |
| `>>` | stdout to the end of a file |
| `2>` | stderr to a file, replacing that file first |

This lesson uses one operator at a time except for the two commands joined by one pipe. More advanced stream combinations are deliberately deferred.

## Pipes: stdout becomes stdin

A **pipeline** joins commands so that data can be processed in stages:

```text
producer | consumer
```

By default, `|` takes the producer's stdout and supplies it as the consumer's stdin.

The first activity command is:

```console
$ grep '^READY' records.txt | wc -l
3
```

Trace it from left to right:

1. `grep '^READY' records.txt` reads `records.txt`.
2. `grep` selects three lines beginning with `READY`.
3. Instead of appearing on screen, those three lines leave `grep` through stdout.
4. `|` carries that stdout to `wc -l` as stdin.
5. `wc -l` counts the three incoming lines.
6. `wc` writes `3` to its own stdout.
7. Because the final stdout is not redirected, `3` appears on screen.

The pipe does not create an intermediate file. The selected lines flow directly between running commands.

### The pipe does not carry stderr by default

The ordinary `|` operator connects stdout only. If the left command writes a diagnostic to stderr, that diagnostic ordinarily remains directed to the terminal rather than becoming input to the right command.

This distinction is why the student must name the stream, not merely say “the output.”

### Avoid an unnecessary `cat`

`grep` can read `records.txt` itself. Prefer:

```text
grep '^READY' records.txt | wc -l
```

over:

```text
cat records.txt | grep '^READY' | wc -l
```

The second form adds a stage without adding meaning. A clear pipeline uses each stage for a necessary transformation.

## Redirect stdout with `>`

The operator `>` routes stdout into a file:

```text
command > destination
```

In the activity:

```console
$ grep '^READY' records.txt > ready.txt
```

`grep` still reads `records.txt` and writes three selected lines to stdout. The shell routes that stdout to `ready.txt`, so the selected lines do not ordinarily appear on screen.

After the command, `ready.txt` must contain exactly:

```text
READY compass
READY maps
READY first-aid
```

The source `records.txt` remains unchanged. Redirection changes where stdout goes; it does not turn `grep` into an editor of its input file.

## The overwrite risk of `>`

A single `>` is destructive to the **destination's old content**. If the destination exists, the shell opens it for replacement and truncates it before the command writes new output.

The activity first creates `ready.txt` with two disposable `WAIT` lines. The next task deliberately uses `>` again on that known-safe destination. This makes replacement visible: the `WAIT` lines disappear and three `READY` lines take their place.

Before pressing Enter on any real command containing `>`, pause and check:

1. Which stream will be redirected?
2. What exact destination path follows `>`?
3. Does that destination already exist?
4. Is its current content disposable or backed up?
5. Could the destination accidentally be the source file?
6. What should the destination contain when the command succeeds?

A serious danger is redirecting to a file that the command also needs to read. For example, do **not** run:

```text
grep '^READY' records.txt > records.txt
```

The shell may truncate `records.txt` before `grep` reads it. The source can become empty. This lesson permits `>` only with the explicitly disposable `ready.txt` and `errors.txt` fixtures.

A misspelled command does not necessarily protect the destination. Shell redirection happens as part of setting up the command, so an existing destination may be truncated even when the program cannot later produce the intended output. Correct path review comes before execution.

## Append stdout with `>>`

The operator `>>` also routes stdout to a file, but it opens the destination for **append**:

```text
command >> destination
```

In the activity:

```console
$ grep '^WAIT' records.txt >> ready.txt
```

The two matching lines are added after the existing three `READY` lines. The final `ready.txt` must be:

```text
READY compass
READY maps
READY first-aid
WAIT batteries
WAIT water
```

`>>` preserves earlier bytes and writes new stdout at the end. It does not:

- sort old and new records;
- merge related lines;
- remove duplicates;
- insert a heading automatically; or
- verify that appending is logically correct.

The student remains responsible for choosing the correct destination and order.

### Choosing between `>` and `>>`

Ask what should happen to existing destination content:

- **Build a fresh report and discard the old report:** use `>` only after confirming replacement is safe.
- **Extend a report and keep what is already there:** use `>>`.

The number of greater-than signs is a meaningful safety decision, not cosmetic punctuation.

## Redirect stderr with `2>`

The fixture includes a small executable named `check-sensor`; `errors.txt` is initially absent so `2>` creates it. Its behavior is fixed:

- stdout: `checking sensor`
- stderr: `sensor unavailable`
- exit status: 1

Run:

```console
$ ./check-sensor 2> errors.txt
checking sensor
```

Trace the streams:

1. `./check-sensor` writes `checking sensor` to stdout.
2. No stdout redirection is present, so that line appears on screen.
3. The helper writes `sensor unavailable` to stderr.
4. `2>` routes stderr to `errors.txt`, replacing its old warning content.
5. The diagnostic therefore does not appear on screen.
6. The command exits with status 1, as expected for the unavailable sensor.

The final `errors.txt` must contain exactly:

```text
sensor unavailable
```

The `2` must touch the operator:

```text
2> errors.txt
```

It names descriptor 2. A plain `> errors.txt` would redirect stdout instead, which is not the task.

## Output location and exit status answer different questions

Redirection answers:

> Where does this stream go?

Exit status answers:

> Did the command report success or failure?

The helper returns status 1 while still producing both stdout and stderr. Redirecting stderr does not force success, suppress the status, or undo output already written. Likewise, a nonzero status does not mean the redirected diagnostic file should be empty.

A later lesson treats exit status in depth. Here the essential point is that stream routing and command success are separate properties.

## Fixture map

The activity opens in `workspace/streams-lab`:

```text
streams-lab/
├── check-sensor
└── records.txt

`ready.txt` and `errors.txt` are absent initially and are created during the activity.
```

### Immutable sources

`records.txt` is read-only and contains:

```text
READY compass
WAIT batteries
READY maps
HOLD radio
WAIT water
READY first-aid
```

`check-sensor` is executable and contains a fixed helper script. Neither source may be edited, renamed, removed, or replaced.

### Disposable destinations

`ready.txt` and `errors.txt` do not exist initially. The student creates both through redirection. After `ready.txt` is created with two `WAIT` lines, it becomes an explicitly disposable existing destination for the overwrite demonstration. These are the only paths the student may create or change.

### Required final state

At completion, `ready.txt` must be exactly:

```text
READY compass
READY maps
READY first-aid
WAIT batteries
WAIT water
```

At completion, `errors.txt` must be exactly:

```text
sensor unavailable
```

`records.txt` and `check-sensor` must remain byte-for-byte unchanged, and the current directory must remain `workspace/streams-lab`.

## Guided practice

Complete five tasks in order. Before each command, say:

> “The producer is ___. It reads from ___. Its stdout goes to ___. Its stderr goes to ___. Existing destination content will be preserved/replaced/not involved. I expect ___ on screen and ___ in files.”

### 1. Count through a pipe

Run:

```console
$ grep '^READY' records.txt | wc -l
3
```

Required explanation:

- `grep` produces three matching lines on stdout;
- `|` sends those lines to `wc` as stdin;
- `wc -l` produces the count on its stdout;
- only the final count appears on screen; and
- no destination file is involved.

### 2. Create a report with stdout redirection

Confirm that `ready.txt` is absent, then run:

```console
$ grep '^WAIT' records.txt > ready.txt
```

Required explanation:

- stdout goes to `ready.txt` rather than the terminal;
- because the destination is absent, `>` creates it;
- the file contains two WAIT lines in source order; and
- `records.txt` remains unchanged.

### 3. Replace a safe existing destination

Acknowledge that the two WAIT lines in `ready.txt` are disposable, then run:

```console
$ grep '^READY' records.txt > ready.txt
```

Required explanation:

- one `>` truncates the existing destination first;
- the two WAIT lines disappear;
- three READY lines take their place; and
- the same operation would be dangerous on important data.

### 4. Append more stdout

Run:

```console
$ grep '^WAIT' records.txt >> ready.txt
```

Required explanation:

- stdout again goes to `ready.txt`;
- `>>` appends instead of truncating;
- the three READY lines remain;
- the two WAIT lines follow them; and
- using `>` would incorrectly erase the READY section.

### 5. Separate stderr

Run:

```console
$ ./check-sensor 2> errors.txt
checking sensor
```

Required explanation:

- stdout remains on screen;
- descriptor 2 identifies stderr;
- stderr replaces the old content of `errors.txt`;
- status 1 is expected; and
- the final report files and source fixtures have their required exact content.

## Vocabulary

- **stream** — a sequence of data read or written by a process.
- **standard input (stdin)** — the default input stream, descriptor 0.
- **standard output (stdout)** — the default ordinary-result stream, descriptor 1.
- **standard error (stderr)** — the default diagnostic stream, descriptor 2.
- **file descriptor** — a small number by which a process and shell identify an open stream; the standard descriptors are 0, 1, and 2.
- **pipe** — the shell operator `|` that connects left-side stdout to right-side stdin.
- **pipeline** — two or more commands connected by pipes.
- **producer** — a command whose output supplies data to another stage or destination.
- **consumer** — a command that reads data produced by an earlier stage.
- **redirection** — shell syntax that changes where a stream comes from or goes.
- **`>`** — stdout redirection that replaces an existing destination.
- **truncate** — reduce a file to zero length before new content is written.
- **overwrite** — replace existing file content with new content.
- **`>>`** — stdout redirection that appends at the destination's end.
- **append** — add content after existing content without removing it first.
- **`2>`** — stderr redirection that replaces an existing destination.
- **destination** — the file receiving a redirected stream.
- **exit status** — a command's numeric success or failure result, separate from stream routing.

## Mastery evidence and contract limits

The activity uses five ordered tasks and exact final-state checks:

1. Exact final stdout `3`, unchanged `records.txt`, absence of `ready.txt`, and cwd evidence support the simple-pipeline task.
2. Exact two-line `ready.txt`, preserved source content, and cwd evidence prove stdout redirection can create a report.
3. Exact three-line `ready.txt`, preserved source content, and cwd evidence prove safe replacement of a known disposable destination.
4. Exact five-line `ready.txt`, preserved source content, and cwd evidence prove ordered append behavior.
5. Exact helper execution with status 1, exact stdout, exact `errors.txt`, exact final `ready.txt`, preserved source/helper content, and final cwd prove separation of stdout and stderr.

The current `pipeline_output` check observes the latest stdout but does not prove the pipeline's internal syntax. The first task therefore also relies on constrained instructions, fixture preservation, and tutor explanation. The contract presently lacks a pipeline-structure predicate and a direct stderr observation check.

For redirection tasks, persistent exact file content is stronger evidence than screen output alone. However, exact content cannot prove every shell spelling that might produce it. Tutor review must require the requested `>`, `>>`, and `2>` syntax and the student's accurate stream trace.

`command_properties` can observe the final helper executable, arguments, and status, but shell redirection tokens are interpreted before the executable receives arguments. The final exact diagnostic file and stdout checks establish the required separation under the current contract.

Conceptual explanations are oral evidence because the v1 activity contract has no rubric-backed short-answer task. A parent or tutor should not grant full mastery based only on matching files.

## Parent or tutor review

Ask the student to answer without running additional commands:

1. What are the names and descriptor numbers of the three standard streams?
2. Where do stdin, stdout, and stderr normally connect in an interactive terminal?
3. Why can stdout and stderr look combined even though they are separate?
4. In `grep '^READY' records.txt | wc -l`, which command is the producer?
5. Which command is the consumer?
6. What exact data crosses the pipe?
7. Does ordinary `|` send stderr into the next command?
8. Why does only `3` appear on screen in the first task?
9. Why is `cat records.txt | grep ...` unnecessary here?
10. What does `>` do to an existing destination before new output is written?
11. Why is `>` risky even if the intended command later fails?
12. Why must the destination path be inspected before pressing Enter?
13. What could happen with `grep ... records.txt > records.txt`?
14. Why was `ready.txt` safe to overwrite in this lesson?
15. What does `>` do when the destination does not yet exist?
16. What is the exact difference between `>` and `>>`?
17. Does `>>` sort or deduplicate appended records?
18. Why would `>` be wrong in the append task?
19. What does the `2` mean in `2>`?
20. Why did `checking sensor` remain on screen?
21. Why did `sensor unavailable` not remain on screen?
22. Did redirecting stderr change the helper's exit status?
23. Can a command produce stdout, stderr, and a nonzero status in one run?
24. Which files were allowed to be created or changed?
25. Which checks prove that the sources were preserved?
26. State the exact required final content of both destination files.

Require precise nouns—stdin, stdout, stderr, source, destination, replace, append—rather than accepting “it sends stuff somewhere.”

## Common misconceptions

- **“A command has one output.”** Stdout and stderr are separate streams with different purposes.
- **“The numbers 0, 1, and 2 count lines.”** They are standard file descriptor identifiers.
- **“A pipe sends everything from the left command.”** Ordinary `|` sends stdout, not stderr.
- **“The right command reads the original file.”** In this pipeline, `wc` reads grep's stdout from stdin.
- **“A pipe saves an intermediate file.”** Data flows directly; no intermediate file is created here.
- **“`>` adds to a file.”** One `>` replaces existing destination content.
- **“`>` waits to see whether the command succeeds.”** The shell prepares redirection first, so truncation can occur before useful output exists.
- **“The source file changes when output is redirected.”** The destination changes; `grep` continues to read the source.
- **“`>>` is always safer.”** It preserves old bytes but can still append to the wrong file or duplicate unwanted data.
- **“Two greater-than signs mean redirect stderr.”** `>>` means append stdout; `2>` means replace a file with stderr.
- **“`2>` redirects two outputs.”** The `2` names stderr.
- **“No terminal output means the command did nothing.”** Stdout may have gone to a file.
- **“Redirecting an error makes the command successful.”** Routing and exit status are independent.
- **“A diagnostic file should be empty if status is nonzero.”** Nonzero status often accompanies useful stderr text.

## Tutor strategy and constraints

For each task:

1. Ask the student to identify every command involved.
2. Ask what each command reads.
3. Ask what each command writes to stdout.
4. Ask whether stderr is expected and where it goes.
5. Ask whether an operator connects commands or targets a file.
6. If a file is targeted, ask whether existing content will be replaced or preserved.
7. Require a prediction of screen output and exact file effects.
8. Give hints in order and reveal the exact command only at level 3.
9. After execution, compare the prediction with observed output and file state.
10. Require the student to explain the safety implication of the operator used.

Keep coaching within these exact commands:

```text
grep '^READY' records.txt | wc -l
grep '^WAIT' records.txt > ready.txt
grep '^READY' records.txt > ready.txt
grep '^WAIT' records.txt >> ready.txt
./check-sensor 2> errors.txt
```

Do not introduce:

- `tee`;
- input redirection `<`;
- descriptor duplication such as `2>&1`;
- combined redirection such as `&>`;
- here-documents or here-strings;
- process substitution;
- command substitution;
- `xargs`;
- variables, loops, aliases, or functions;
- command chaining with `;`, `&&`, or `||`;
- background jobs; or
- advanced pipelines.

If a result differs, inspect in this order:

1. current working directory;
2. exact source and destination spelling;
3. quoted grep pattern and capitalization;
4. operator shape: `|`, `>`, `>>`, or `2>`;
5. command order around the pipe;
6. current content of `ready.txt` and `errors.txt`; and
7. fixture integrity.

Because the tasks intentionally build on destination state, reset and repeat from task 1 if `ready.txt` was truncated or appended incorrectly. Reset immediately if either immutable source changes or an unrequested file is created.

## Safety and craftsmanship

- Stay in `workspace/streams-lab`.
- Read a command from left to right before executing it.
- Name the exact stream routed by each operator.
- Check every destination path twice before using `>` or `2>`.
- Use `>` only on the explicitly disposable fixture.
- Use `>>` only when preserving prior content is part of the plan.
- Never redirect a command's output onto an important input file.
- Do not assume an empty screen means failure.
- Verify redirected content exactly, including order and line breaks.
- Preserve `records.txt` and `check-sensor` byte-for-byte.
- Change only `ready.txt` and `errors.txt`.
- Prefer a small, legible pipeline in which every stage has a clear job.
