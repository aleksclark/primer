# Lesson 16: Your first Bash script

## Objectives

By the end of this lesson, the student can:

- describe a script as a text file containing commands for an interpreter;
- place `#!/bin/bash` on the first line of a Bash script and explain its purpose;
- write a useful comment for human readers and distinguish it from the shebang;
- combine two familiar, simple commands into a small repeatable procedure;
- inspect a script before granting it execute permission;
- use `chmod u+x` to add only user execute permission;
- run a script in the current directory with `./first-report.sh`;
- explain why the script creates the same predictable output on repeated runs; and
- verify the script's structure, mode, and generated output while preserving supplied material.

## Standards

Scripting-specific digital-literacy standards are pending. This lesson uses the closest current file standards provisionally.

- **Primary — `PRIMER.DL.6.FILES.1`:** Create and organize a small workspace. The student creates a script, and the script creates one output directory and one report at explicit relative paths.
- **Reinforcement — `PRIMER.DL.6.FILES.2`:** Inspect and confirm expected text. The student reads the script before execution and verifies an exact generated report.

The activity records `standardsStatus: pending-scripting-standards` so the temporary alignment is explicit.

## Vocabulary

| Term | Meaning in this lesson |
|---|---|
| script | A text file containing commands that an interpreter can read and execute in order. |
| Bash | The shell and command interpreter used for this script. |
| interpreter | A program that reads instructions and carries them out. |
| shebang | The first-line `#!` directive that names the interpreter for direct execution. |
| comment | Explanatory text for human readers that Bash ignores as an instruction. |
| execute permission | File metadata that permits a regular file to be launched as a program. |
| relative path | A path interpreted from the current working directory, such as `./first-report.sh`. |
| generated output | A file created by running a procedure rather than supplied as source material. |
| predictable | Producing the same intended final state from the same starting conditions. |

## From individual commands to a procedure

At the prompt, a student has run one command at a time. A script records a sequence so it can be inspected, reused, and improved.

A script is still ordinary text. Adding execute permission does not compile it or hide its contents. The interpreter reads the lines and performs the commands in order.

The complete script in this lesson is deliberately small:

```bash
#!/bin/bash
# Create a small, predictable field report for human readers.
mkdir -p output
printf 'System check: ready\nFiles are organized\n' > output/first-report.txt
```

Every line has one clear responsibility. The student must explain all four lines rather than treating the file as magic.

## The shebang selects Bash

The first line is:

```bash
#!/bin/bash
```

`#!` is called a **shebang**. During direct execution, the operating system uses the path after it to select the interpreter. Here, `/bin/bash` reads the remaining script lines.

The position matters:

- the shebang must be the first line;
- no blank line or other text comes before it;
- this lesson requires the exact interpreter path `/bin/bash`; and
- direct execution with `./first-report.sh` makes the shebang relevant.

Running `bash first-report.sh` would name Bash explicitly and bypass the lesson's direct-execution demonstration. It is therefore not the requested command.

## Comments serve human readers

The second line is:

```bash
# Create a small, predictable field report for human readers.
```

Bash treats an ordinary line beginning with `#` as a comment and does not execute it as a command. That does not make comments unimportant. A useful comment helps a future reader understand purpose, reason, or a non-obvious constraint.

The comment does not merely say “run `mkdir`” or repeat the next line. It explains the script's purpose: producing a small, predictable report.

The shebang and the comment both begin with `#`, but they are not interchangeable:

- `#!/bin/bash` is a first-line interpreter directive;
- `# Create ...` is explanatory prose for people.

## Two simple commands do the work

### Ensure the output directory exists

The third line is:

```bash
mkdir -p output
```

`mkdir` creates a directory. The `-p` option makes this bounded use repeatable:

- on the first run, it creates `output`;
- on a later run, an existing `output` directory is accepted rather than causing an error.

The path is relative to the script's current working directory in this controlled activity. The student starts and remains in `workspace/script-lab`, so the expected directory is `workspace/script-lab/output`.

### Write one exact report

The fourth line is:

```bash
printf 'System check: ready\nFiles are organized\n' > output/first-report.txt
```

`printf` produces two lines. The `\n` sequences become newline characters when `printf` runs. The shell's `>` operator sends stdout to `output/first-report.txt`.

The destination is explicit and disposable. As learned in lesson 13, `>` replaces an existing destination. This is intentional here:

- the first run creates the report;
- each later run replaces it with the same two lines;
- repeated runs do not accumulate duplicates.

The exact report is:

```text
System check: ready
Files are organized
```

The script does not need variables, arguments, conditions, loops, pipelines, or advanced shell features. Those belong to later lessons.

## Write text without executing it

The activity uses one controlled `printf` command to write the four script lines:

```console
$ printf '%s\n' '#!/bin/bash' '# Create a small, predictable field report for human readers.' 'mkdir -p output' "printf 'System check: ready\\nFiles are organized\\n' > output/first-report.txt" > first-report.sh
```

There are two different levels of output here:

1. The **outer** `printf` runs now and writes the script text into `first-report.sh`.
2. The **inner** `printf` appears as text on the script's fourth line and runs only later, when the script executes.

Therefore, writing the script must not create `output/first-report.txt`. If the report appears before the script is run, the command was not entered as specified.

## Inspect before granting execution

Before making a new script executable, read it:

```console
$ cat first-report.sh
```

Inspection is a craftsmanship and safety habit. Check:

1. Is the exact shebang the first line?
2. Does the comment explain the script's purpose?
3. Are all commands understood?
4. Are all source and destination paths expected?
5. Does any redirection replace a file, and is that destination safe?

A script should never receive trust merely because its filename ends in `.sh`. The extension is a naming convention; its text and permissions determine what it is and how it can be used.

## Add only the needed execute permission

A newly created text file normally starts with mode `0644` in this activity:

```text
user:  read, write
group: read
other: read
```

Add user execute permission with a narrow symbolic change:

```console
$ chmod u+x first-report.sh
```

The resulting mode is `0744`:

```text
user:  read, write, execute
group: read
other: read
```

`chmod u+x` changes permission metadata and preserves the script text. It does not run the script. Broad modes such as `chmod 777` grant unrelated permissions and are not appropriate.

Recall the type distinction from lesson 10:

- execute on a regular script file permits it to be launched;
- execute on a directory permits traversal.

## Run by relative path

Run the executable file in the current directory with:

```console
$ ./first-report.sh
```

The `./` prefix is a relative path:

- `.` means the current directory;
- `/` separates path components;
- `first-report.sh` names the file.

A bare `first-report.sh` ordinarily asks Bash to search `PATH`; the current directory is not assumed to be in that search. `./first-report.sh` names the intended file directly.

The execution sequence is:

1. Bash receives the relative executable path.
2. The operating system checks execute permission.
3. The operating system reads `#!/bin/bash`.
4. `/bin/bash` interprets the remaining lines.
5. `mkdir -p output` ensures the directory exists.
6. `printf ... > output/first-report.txt` replaces the report with two exact lines.
7. The script exits successfully and the prompt returns.

The terminal may show no ordinary output because the script redirects its final command's stdout into a file. Verify the filesystem rather than assuming silence means failure.

## Predictability and repeated runs

A small script should have an understandable relationship between starting state and final state.

This script is predictable because:

- all paths are explicit and relative to the assigned workspace;
- `mkdir -p` accepts either an absent or existing `output` directory;
- `printf` emits fixed text;
- `>` replaces the known generated report rather than appending; and
- no time, user input, network data, variables, or changing system information affects the result.

After any successful run, the final report is exactly:

```text
System check: ready
Files are organized
```

A second run leaves the same final content. This does not mean the script did nothing; it means the procedure converged on the same intended state.

## Fixture map

The activity begins in:

```text
workspace/script-lab/
└── README.txt
```

`README.txt` is read-only and contains:

```text
Build one small script here. Keep this note unchanged.
```

After completion, the intended tree is:

```text
workspace/script-lab/
├── README.txt
├── first-report.sh
└── output/
    └── first-report.txt
```

Only `first-report.sh`, `output`, and `output/first-report.txt` are student-created. `README.txt` remains unchanged.

## Guided practice

Complete the four tasks in order. Before each command, state:

1. Which file or permission metadata can change?
2. Which paths should remain absent or unchanged?
3. Is the command writing script text, inspecting text, changing mode, or executing commands?
4. What exact state should the verifier observe afterward?

### 1. Write the script

Run:

```console
$ printf '%s\n' '#!/bin/bash' '# Create a small, predictable field report for human readers.' 'mkdir -p output' "printf 'System check: ready\\nFiles are organized\\n' > output/first-report.txt" > first-report.sh
```

Expected state:

- `first-report.sh` contains exactly four lines;
- its mode is `0644`;
- `output/first-report.txt` does not exist; and
- `README.txt` is unchanged.

### 2. Inspect the script

Run:

```console
$ cat first-report.sh
```

Read and explain each line. Do not add execute permission until the interpreter, purpose, commands, and destination are all understood.

### 3. Add user execute permission

Run:

```console
$ chmod u+x first-report.sh
```

Expected state:

- the script text is unchanged;
- its mode is `0744`; and
- the report still does not exist because permission changes do not execute commands.

### 4. Run and verify

Run:

```console
$ ./first-report.sh
```

Expected generated file:

```text
System check: ready
Files are organized
```

Explain the role of `./`, the shebang, each command, `>`, and the execute bit. Then explain why another run would leave the same final report.

## Mastery check

The student has mastered this lesson when they can, without copying an unexplained answer:

1. define script, interpreter, shebang, comment, and execute permission;
2. explain why `#!/bin/bash` must be first;
3. distinguish the shebang from the ordinary purpose comment;
4. read the script line by line and predict every filesystem change;
5. explain why writing the script does not run its inner commands;
6. inspect the script before changing permissions;
7. justify `chmod u+x` and reject broader unrelated permission grants;
8. explain why `./first-report.sh` is used instead of a bare filename;
9. verify that the script remains mode `0744` and unchanged in content;
10. verify the exact two-line generated report; and
11. explain why repeated runs do not duplicate output.

## Parent/tutor prompts

Ask the student:

- “Which part of this file is for the operating system, and which part is for a human reader?”
- “What makes the fourth line text while you create the script, but a command when the script runs?”
- “What changed when you ran `chmod u+x`? What did not change?”
- “Why does the shell need `./` here?”
- “Where did the script's ordinary output go?”
- “What would change if `>` were replaced with `>>`?”
- “Why is this particular overwrite safe, and why must that judgment not be generalized to important files?”
- “What exact evidence shows that a second run is predictable?”

Do not advance merely because the generated file matches. Require the student to explain the mechanism and the safety boundaries.

## Safety boundaries and deferred topics

This lesson intentionally excludes:

- scripts copied from the internet or run outside the assigned workspace;
- editors and advanced script-construction methods;
- variables and positional arguments;
- conditions, loops, and functions;
- command substitution and process substitution;
- pipelines and background execution inside scripts;
- broad numeric permission modes;
- privileged execution; and
- generated output based on changing system information.

Lessons 17 and 18 introduce variables, arguments, conditions, and bounded loops. This lesson establishes the smaller foundation: readable text, an explicit interpreter, understood commands, narrow permissions, direct relative execution, and verifiable output.

## Verification design

The activity uses complementary checks rather than trusting only one final file:

1. `content_match` anchors the full script shape, including the first-line shebang, exact purpose comment, simple commands, relative destination, and final newline.
2. `path_mode` proves the script begins as ordinary `0644` text and becomes exactly `0744` after `chmod u+x`.
3. `file_not_exists` proves writing and permission changes do not prematurely generate the report.
4. `command_properties` records the required read-only inspection and final direct script execution.
5. `path_type`, `content_equals`, and `content_match` verify that generated output is an ordinary file with the exact two-line format.
6. Repeated content, mode, immutable-fixture, and current-directory checks preserve the activity's safety boundaries.

`content_match` is used deliberately for structural assertions, while `content_equals` independently enforces exact generated text. `path_mode` compares permission bits using the activity contract's four-digit octal strings; students use symbolic `chmod` rather than numeric modes.

## Parent review note

This is the first lesson in which the student creates an executable procedure. Favor restraint over cleverness. The instructional achievement is not “I can paste shell syntax.” It is “I can account for every line, every permission, every path, and every generated byte.” If the student cannot explain why the file is safe to run and why its result is predictable, reset and repeat the inspect-predict-execute-verify cycle.
