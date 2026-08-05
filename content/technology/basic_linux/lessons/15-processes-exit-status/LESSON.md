# Lesson 15: Commands, processes, and exit status

## Objectives

By the end of this lesson, the student can:

- distinguish a shell builtin from an external executable program;
- explain why one command name, such as `true`, can have both builtin and external implementations;
- describe `PATH` as an ordered list of directories used to locate external commands;
- describe environment variables as named text values associated with a process and normally inherited by child processes;
- distinguish a program on disk from a running process;
- model the interactive Bash shell as a parent process that can start and wait for a child process;
- explain that a finished command returns an integer exit status;
- interpret status `0` as conventional success and a nonzero status as failure;
- use `$?` to observe only the immediately previous command's status; and
- preserve immutable input while creating four exact, deterministic reports.

## Standards

These references are provisional while process, shell, environment, and exit-status standards are pending.

- **Primary — `PRIMER.DL.6.NAV.1`:** Use basic shell commands within an assigned workspace. The student remains in the supplied lab while reasoning accurately about how the shell resolves and runs commands.
- **Reinforcement — `PRIMER.DL.6.FILES.2`:** Inspect and confirm expected text. The student creates and verifies four deterministic text reports while preserving the supplied read-only note.

The activity records `standardsStatus: pending-new-standards` so this temporary alignment is explicit.

## Vocabulary

| Term | Meaning in this lesson |
|---|---|
| command | An instruction the shell interprets, including a name and any arguments or shell syntax. |
| shell builtin | A command implemented inside the shell itself. |
| external program | Executable code stored in a file and started as a process. |
| `PATH` | An environment variable containing an ordered, colon-separated list of directories searched for external command names. |
| environment | Named text values associated with a process and normally inherited by its children. |
| program | Executable instructions stored on disk. |
| process | A running instance of a program or shell, with runtime state such as its environment and current directory. |
| parent process | A process that starts another process. |
| child process | A process started by another process. |
| exit status | An integer returned when a command finishes; `0` conventionally means success and nonzero means failure. |
| `$?` | A Bash parameter that expands to the immediately previous command's exit status. |

## Commands are not all the same kind

At a prompt, the first word often looks like the name of a program. Sometimes it is. Sometimes the command is built into Bash.

### Shell builtins

A **builtin** is implemented by the shell. Bash does not need to locate and start a separate executable file to perform it. `cd` is the clearest example: changing directory must affect the current shell's own working directory. If an unrelated child process changed *its* directory, the parent shell would remain where it was.

Bash also supplies a builtin named `true`. The runtime contains an external `true` program too, but ordinary Bash lookup selects its builtin. A name can therefore have more than one possible implementation.

### External programs

An **external program** is executable code stored in a file. In this lesson, `env` is external. When Bash resolves and executes it, the operating system starts a process from that executable.

The activity uses Bash's `type -t` builtin inside a bounded child shell:

```console
$ bash -c 'type -t cd; type -t true; type -t env' > command-kinds.txt
```

The report is:

```text
builtin
builtin
file
```

The lines correspond, in order, to `cd`, `true`, and `env`.

This does **not** mean every command is always exactly one kind. Shell lookup can involve aliases, functions, builtins, explicit paths, and executable files. This lesson concentrates on builtins and external programs.

## How `PATH` helps locate external commands

When a command name has no slash and Bash needs an external program, it consults `PATH`. A typical value is a colon-separated list:

```text
/some/bin:/another/bin:/third/bin
```

Bash searches those directories from left to right for a suitable executable name. The first applicable result wins.

Important distinctions:

- `PATH` is not the current working directory.
- `PATH` does not locate a builtin such as `cd`; the shell handles builtins through its own lookup rules.
- An explicit path such as `/bin/sh` or `./helper` names a location directly instead of requesting ordinary `PATH` lookup.
- Changing `PATH` changes command lookup for the affected process and its inheriting children; it does not move files or change directory.

The activity does not depend on the host's varying `PATH`. Instead it creates one controlled observation:

```console
$ env -i PATH=/lesson/bin /bin/sh -c 'printf "PATH=%s\n" "$PATH"' > path-report.txt
```

Read it from outside inward:

1. Bash locates and starts external `env` using its existing environment.
2. `env -i` begins a replacement environment with no inherited entries.
3. `PATH=/lesson/bin` adds one named value.
4. `env` starts `/bin/sh` by explicit path with that environment, so the artificial `PATH` is not used to locate it.
5. The child `sh` expands its inherited `$PATH` and prints it.

The exact report is:

```text
PATH=/lesson/bin
```

`/lesson/bin` is intentionally artificial. It is evidence of inheritance, not a directory used to find the explicitly named `/bin/sh` in this experiment.

## Environment variables belong to processes

An environment variable is a named text value available to a process. `PATH` is one environment variable, but “environment” and “PATH” are not synonyms. A process may have many environment entries.

Children normally inherit copies of the parent's environment. A child can receive additions, removals, or replacements, as `env -i PATH=/lesson/bin ...` demonstrates. Changing one child's environment does not retroactively rewrite the parent's environment.

Environment variables are not universal permanent values shared identically by every process. They are process state passed across a parent-child boundary according to the launch request.

## A program is not a process

A **program** is executable instructions stored on disk. A **process** is a running instance with live state. The distinction is like a written recipe versus one particular cooking session:

- one program can be run many times;
- each run is a separate process instance;
- each process has its own current directory and environment;
- processes have identifiers and parent relationships; and
- each process eventually continues, waits, stops, or exits according to the operating system and its program.

The interactive Bash is itself a process. When it executes an external `sh`, Bash is the parent and `sh` is a child. Bash ordinarily waits for the foreground child to finish. The child exits, Bash regains control, and the prompt returns.

The deterministic observation is:

```console
$ sh -c 'printf "child process ran\n"' > process-report.txt
```

The child writes:

```text
child process ran
```

Then it exits and the interactive prompt returns.

### Why there is no `ps` task

`ps` commonly displays a process-table snapshot, including process identifiers and relationships. It is unavailable in the declared `coreutils-basic` runtime. Do not attempt it, install it, or substitute an unapproved process-inspection tool.

The process concept is taught through the controlled child lifecycle:

1. the parent Bash accepts a command;
2. Bash starts a child `sh` process;
3. the child performs its work;
4. the child exits;
5. Bash receives completion and presents another prompt.

Process identifiers, scheduling, process tables, and signals remain discussion concepts here. No PID should be invented or claimed as observed.

## Exit status communicates success or failure

When a command finishes, it returns an integer **exit status** to its parent shell.

By Unix convention:

- `0` means success;
- any nonzero value means failure.

Nonzero does **not** always mean `1`. The `false` command returns `1`, but other programs use other nonzero values. Consult a command's documentation when the exact value matters.

Exit status is separate from output:

- stdout is ordinary output text;
- stderr is diagnostic output text;
- exit status is an integer completion result.

A command can print nothing and succeed. It can print useful output and fail. Redirection changes where output goes; it does not itself turn output into an exit status.

## `$?` remembers only the immediately previous status

In Bash and `sh`, `$?` expands to the status of the immediately previous command. It is temporary evidence, not a permanent command history.

Consider:

```sh
true
printf 'true=%s\n' "$?"
```

`true` returns `0`. Because `printf` expands `$?` immediately afterward, it records:

```text
true=0
```

Now consider:

```sh
false
printf 'false=%s\n' "$?"
```

`false` returns `1`, so the immediate report is:

```text
false=1
```

But this sequence demonstrates replacement:

```sh
false
true
printf 'latest=%s\n' "$?"
```

`false` first sets the remembered status to `1`. Then `true` runs and replaces it with `0`. By the time `printf` expands `$?`, the latest command is `true`, so the report is:

```text
latest=0
```

If a status matters, inspect or save it before running another command. This lesson only observes it immediately; later scripting lessons introduce variables and control flow.

## Quoting in the controlled experiments

The `sh -c` command receives one string to interpret. The outer single quotes are important:

```text
sh -c '... "$?" ...'
```

They prevent the interactive parent Bash from expanding `$?` too early. The text reaches the child `sh`, and that child expands `$?` at the intended point after `true` or `false` runs.

Likewise:

```text
sh -c '... "$PATH" ...'
```

lets the child expand its inherited `PATH` rather than letting the parent substitute its own value first.

This is a bounded quoting observation, not a complete shell-scripting lesson.

## Fixture map

The activity opens in:

```text
workspace/process-lab/
└── README.txt
```

`README.txt` is read-only and contains:

```text
Process lesson workspace. Keep this file unchanged.
```

The student creates exactly four reports:

```text
workspace/process-lab/
├── README.txt
├── command-kinds.txt
├── path-report.txt
├── process-report.txt
└── status-report.txt
```

No other file is needed.

## Guided practice

Complete the four tasks in order. Before each command, state:

1. Which shell interprets the command line?
2. Which named command is a builtin and which is an external program?
3. Is a child process started?
4. What exact text should be written?
5. What exit status should the final command return?

### 1. Classify builtins and programs

Run:

```console
$ bash -c 'type -t cd; type -t true; type -t env' > command-kinds.txt
```

Expected `command-kinds.txt`:

```text
builtin
builtin
file
```

Explain:

- `cd` is implemented by Bash;
- Bash selects its builtin `true` even though an external implementation exists; and
- `env` resolves to an executable file.

### 2. Observe a controlled child environment

Run:

```console
$ env -i PATH=/lesson/bin /bin/sh -c 'printf "PATH=%s\n" "$PATH"' > path-report.txt
```

Expected `path-report.txt`:

```text
PATH=/lesson/bin
```

Explain that `env` creates a controlled environment and starts the child `sh` with it. The child's expansion proves which value it received.

### 3. Start and finish a child process

Run:

```console
$ sh -c 'printf "child process ran\n"' > process-report.txt
```

Expected `process-report.txt`:

```text
child process ran
```

Explain that the interactive Bash starts the child, waits while it runs, receives its completion, and then presents the prompt again.

### 4. Record success, failure, and replacement

Run:

```console
$ sh -c 'true; printf "true=%s\n" "$?"; false; printf "false=%s\n" "$?"; false; true; printf "latest=%s\n" "$?"' > status-report.txt
```

Expected `status-report.txt`:

```text
true=0
false=1
latest=0
```

Trace every line:

1. `true` returns `0`, and the next `printf` records it.
2. `false` returns `1`, and the next `printf` records it.
3. another `false` returns `1`;
4. `true` then replaces the remembered status with `0`; and
5. the final `printf` therefore records `latest=0`.

The final `printf` succeeds, so the bounded child shell itself finishes with status `0`.

## Common mistakes and corrections

### Calling every command a program

Incorrect idea: typing a command name always starts a separate executable.

Correction: Bash can execute a builtin internally. `cd` and the selected `true` are builtins in this activity; `env` is external.

### Assuming a name has only one implementation

Incorrect idea: because an external `true` exists, Bash must run that file.

Correction: names can overlap. Bash's lookup rules select its builtin `true` in the classification experiment.

### Confusing `PATH` with current location

Incorrect idea: `PATH` says which directory the shell is currently in.

Correction: the current working directory answers “where is this process operating?” `PATH` answers “which directories should be searched for external command names?”

### Treating the environment as global

Incorrect idea: changing the child's `PATH` changes every shell on the computer.

Correction: environment values belong to processes. A child normally inherits values, but it can receive a controlled replacement without rewriting its parent.

### Treating program and process as synonyms

Incorrect idea: the file `sh` and one running child shell are the same kind of object.

Correction: `sh` names executable program code; a child process is one active run of that code with its own runtime state.

### Expecting success to print `0`

Incorrect idea: `true` should display `0` on stdout.

Correction: `true` normally prints nothing. It communicates success through exit status. The later `printf` turns that status into visible report text.

### Assuming all failures return `1`

Incorrect idea: every failed command has status `1`.

Correction: every nonzero status represents failure by convention, but commands define their own values. `false` returns `1` in this activity.

### Checking `$?` too late

Incorrect idea:

```sh
false
true
printf '%s\n' "$?"
```

still reports `false`.

Correction: it reports `true`, the immediately previous command. Every completed command supplies a new latest status.

### Confusing status with output

Incorrect idea: redirecting stdout to a report captures the command's exit status automatically.

Correction: redirection captures text. `$?` is shell state and must be expanded into text if the lesson wants it written to a file.

### Attempting `ps`

Incorrect idea: the lesson cannot discuss a process unless `ps` is run.

Correction: `ps` is unavailable. The parent starts a child, the child produces deterministic output and exits, and the prompt returns. That lifecycle supports the bounded process model without a process-table snapshot.

## Deterministic completion checks

The activity checks:

1. `command-kinds.txt` exactly equals `builtin\nbuiltin\nfile\n`;
2. `path-report.txt` exactly equals `PATH=/lesson/bin\n`;
3. `process-report.txt` exactly equals `child process ran\n`;
4. `status-report.txt` exactly equals `true=0\nfalse=1\nlatest=0\n`;
5. selected latest child-shell commands have the expected executable, arguments, and final status;
6. `README.txt` remains byte-for-byte unchanged; and
7. the current directory remains `workspace/process-lab`.

The current contract observes only the latest command for `command_properties`. Redirection syntax is processed by the parent shell and therefore is not part of the child executable's argument list. Filesystem outcomes establish the exact reports, while ordered instructions, hints, and tutor questioning establish the intended interpretation.

The contract also lacks a direct check for each intermediate `$?` expansion or for a process parent-child relation. The bounded `sh -c` argument, exact output, and oral explanation provide the strongest available evidence. This limitation is especially important here because `SCHEMA_ASSESSMENT.md` identifies process and exit-status checks as future contract work.

## Mastery questions

After all deterministic checks pass, ask the student to answer in complete sentences:

1. Why must `cd` affect the current shell rather than an unrelated child process?
2. What is the difference between a shell builtin and an external program?
3. Why can `true` be classified as a builtin even when an external `true` file exists?
4. What does `PATH` contain, and when does the shell use it?
5. How is `PATH` different from the current working directory?
6. What does it mean for a child process to inherit an environment value?
7. What is the difference between a program and a process?
8. In the third task, which process is the parent and which is the child?
9. What does exit status `0` conventionally mean? What does nonzero mean?
10. Why does `false; true; printf ... "$?"` report `0` rather than `1`?
11. How are stdout, stderr, and exit status different?
12. What could `ps` show on a fuller system, and why was it not used here?

Mastery requires accurate distinctions, not merely four matching files.

## Parent or tutor review

Look for the student to:

- say “builtin” versus “external program” precisely;
- avoid claiming that every typed command launches a separate process;
- explain lookup precedence rather than assuming one implementation per name;
- describe `PATH` as ordered directories, not as location or file contents;
- describe environment inheritance as process-specific;
- distinguish stored program code from a running process instance;
- identify the interactive Bash as a process and parent;
- say “zero means success by convention; nonzero means failure,” without claiming all failures are `1`;
- check `$?` immediately and explain why later commands replace it;
- keep output channels separate from exit status; and
- acknowledge that `ps` is unavailable rather than attempting unsupported workarounds.

If the student can reproduce the files but cannot explain these distinctions, reset or repeat the relevant task with oral prediction before execution.

## Safety and scope

This lesson deliberately excludes process control. Do not introduce:

- background jobs or `&`;
- `jobs`, `wait`, or job-control syntax;
- `kill` or signals;
- `top`, `pgrep`, `/proc`, or other inspection substitutions;
- priorities such as `nice` and `renice`;
- package installation; or
- scripts, conditions, loops, or advanced variable handling.

Those topics require additional runtime support and instruction. This lesson's narrow goal is a reliable mental model: the shell resolves commands, processes run, environments are inherited, commands finish, and status reports success or failure.
