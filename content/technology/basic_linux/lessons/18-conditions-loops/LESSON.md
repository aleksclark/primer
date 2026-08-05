# Lesson 18: Conditions and loops

## Objectives

By the end of this lesson, the student can:

- use `test -f "path"` to ask whether a path names a regular file;
- recognize `[ -f "path" ]` as another form of `test` and explain its required spaces;
- explain how a command's success or failure makes `if` choose exactly one branch;
- read and write a bounded `for` loop over a fixed list of known files;
- quote loop values as `"$file"` so spaces and wildcard characters remain data;
- build, inspect, permission, run, and verify a small script with deterministic content and output while preserving supplied files.

## Standards

Scripting-specific digital-literacy standards are pending, so this lesson provisionally uses the closest current file standards.

- **Primary — `PRIMER.DL.6.FILES.1`:** The student maintains a bounded script at an explicit path and uses it to create one organized report.
- **Reinforcement — `PRIMER.DL.6.FILES.2`:** The student safely inspects exact script text and generated results while confirming that supplied files remain unchanged.

The activity records `standardsStatus: pending-scripting-standards` explicitly.

## Core ideas

### `test` asks a question through exit status

The `test` command examines a condition. With `-f`, it asks whether a path exists and is a regular file:

```bash
test -f "inputs/field note.txt"
```

`test` normally prints nothing. It communicates through exit status:

- status `0` means the condition succeeded;
- a nonzero status means the condition failed.

The test is read-only. It does not create, repair, or change the path.

### `[ ]` is a command form, not decorative punctuation

This means the same thing:

```bash
[ -f "inputs/field note.txt" ]
```

The opening `[` is a command name, and the closing `]` is its required final argument. Shell words must remain separated, so these spaces matter:

```bash
[ -f "$file" ]   # correct
[-f "$file"]     # wrong: the shell looks for a different command
[ -f "$file"]    # wrong: the value and closing ] are joined
```

This lesson uses only the regular-file test `-f`. More complex comparisons are unnecessary here.

### `if` selects one branch

Bash conditions are commands:

```bash
if [ -f "$file" ]; then
  printf 'FOUND: %s\n' "$file"
else
  printf 'MISSING: %s\n' "$file"
fi
```

Bash runs the condition after `if`.

- If `[ -f "$file" ]` succeeds, Bash runs the `then` branch and skips `else`.
- If it fails, Bash skips `then` and runs the `else` branch.
- `fi` closes the conditional.

The condition does not need to print the words `true` or `false`. Its exit status controls the choice.

### A fixed `for` list is bounded

A `for` loop repeats its body once for each listed item:

```bash
for file in "inputs/alpha.txt" "inputs/field note.txt" "inputs/missing.txt"
do
  printf '%s\n' "$file"
done
```

The list has exactly three shell words, so the body runs exactly three times and stops. The order is explicit and deterministic. This is different from an unbounded loop and safer for a first automation task than searching an unknown tree or accepting arbitrary input.

The quotes around `"inputs/field note.txt"` make that two-word filename one list item. Without them, the loop would receive `inputs/field` and `note.txt` as separate items.

### Quote every path expansion

Within each iteration, `file` stores one complete path. Use:

```bash
[ -f "$file" ]
printf 'FOUND: %s\n' "$file"
```

Double quotes allow `$file` to expand while preserving the result as one argument. An unquoted expansion can split at spaces or treat wildcard characters as filename patterns. Paths are data; quoting preserves their boundaries.

### Deterministic output and preservation

The script redirects the output of the complete loop:

```bash
done > output/file-status.txt
```

The `>` operator replaces the report at the start of each run. Given unchanged inputs, every run therefore produces the same bytes rather than appending duplicate lines.

Conditions only inspect the source paths. The script reports that `inputs/missing.txt` is absent but never creates it. The two existing input files and `README.txt` remain unchanged.

## The bounded script

The completed `file-status.sh` is exactly:

```bash
#!/bin/bash
# Report whether three known paths are regular files.
mkdir -p output
for file in "inputs/alpha.txt" "inputs/field note.txt" "inputs/missing.txt"
do
  if [ -f "$file" ]; then
    printf 'FOUND: %s\n' "$file"
  else
    printf 'MISSING: %s\n' "$file"
  fi
done > output/file-status.txt
```

Line by line:

1. The shebang selects Bash for direct execution.
2. The comment tells a human reader the script's bounded purpose.
3. `mkdir -p output` ensures the one destination directory exists.
4. `for` supplies exactly three known, quoted relative paths in a fixed order.
5. `do` begins the repeated body.
6. `[ -f "$file" ]` safely tests the current path; `then` begins its success branch.
7. The success branch emits one `FOUND` line with a fixed format.
8. `else` begins the failure branch.
9. The failure branch emits one `MISSING` line without creating the path.
10. `fi` closes the condition.
11. `done` closes the loop, and `>` replaces the single report with all three iterations' output.

There are no wildcards, recursive searches, changing loop bounds, writes to inputs, or infinite control paths.

## Activity sequence

The fixture begins as:

```text
workspace/condition-lab/
├── README.txt                 (read-only; preserve)
└── inputs/
    ├── alpha.txt              (read-only; preserve)
    └── field note.txt         (read-only; preserve)
```

`inputs/missing.txt` is deliberately absent and must stay absent.

Complete five tasks in order:

1. **Practice `test` and `if`.** Run the supplied `bash -c` condition and predict the `FOUND` branch.
2. **Write and inspect.** Create the exact eleven-line script and read it with `cat`; no report exists yet.
3. **Add permission.** Run `chmod u+x file-status.sh`; verify exact mode `0744`, unchanged text, and no report.
4. **Run the bounded loop.** Execute `./file-status.sh`; it performs exactly three iterations.
5. **Verify the result.** Read the report, account for each branch, and confirm source preservation.

The exact generated report is:

```text
FOUND: inputs/alpha.txt
FOUND: inputs/field note.txt
MISSING: inputs/missing.txt
```

The final tree is:

```text
workspace/condition-lab/
├── README.txt
├── file-status.sh             (mode 0744)
├── inputs/
│   ├── alpha.txt
│   └── field note.txt
└── output/
    └── file-status.txt
```

## Safety discipline

This first loop is intentionally narrow:

- work only in `workspace/condition-lab`;
- iterate only over the three literal paths shown in the script;
- never replace the list with `*`, `find`, command substitution, or user input;
- never use `while` or `until` here;
- quote every `"$file"` expansion;
- use fixed `printf` format strings and pass paths as data;
- write only `file-status.sh` and the script-generated `output/file-status.txt`;
- never edit either supplied input, `README.txt`, or create the missing input;
- add only user execute permission with `chmod u+x`.

Bounds make automation reviewable: before execution, a reader can count three iterations, identify every tested path, and identify the only generated destination.

## Mastery check

Require the student to explain, not merely reproduce, these facts:

1. What question does `test -f` ask, and does it change the file?
2. Why does `[ -f "$file" ]` require spaces after `[` and before `]`?
3. How does an exit status make `if` choose `then` or `else`?
4. Why does exactly one branch run per iteration?
5. Why does this loop run exactly three times and then stop?
6. Why is `"inputs/field note.txt"` quoted in the loop list?
7. Why is `"$file"` quoted in both the test and `printf`?
8. Why does the missing path produce a line without being created?
9. Why do writing, reading, and permissioning the script not create the report?
10. Why does another unchanged run produce identical report content?
11. Which files may change, and which supplied paths must remain unchanged?

## Tutor boundaries

Keep instruction within `test -f`, `[ -f ]`, one `if`/`then`/`else`/`fi`, one `for`/`in`/`do`/`done` loop over the exact three paths, quoted expansions, and fixed-format `printf`. Do not introduce `[[ ]]`, arithmetic or compound tests, `case`, functions, `while`, `until`, globbing, `find`, recursion, command substitution, user-driven lists, or advanced expansion.

Give graduated hints in order. Before each command, ask the student to predict the branch, iteration count and order, and which content or metadata can change. Reset after changes outside the lab, altered required script text, broad permissions, edits to immutable fixtures, creation of `inputs/missing.txt`, or unrelated files.

## Verification and contract limits

The activity combines exact script-content checks, exact generated-result checks, mode and path-type checks, immutable-fixture checks, missing-path checks, current-directory checks, latest-command evidence, and exact command-output evidence. Together these prove the bounded final state and required inspect and run commands.

The v1 activity contract observes only the latest command and cannot directly score the student's explanation or record the internal iteration history of an executed script. Exact script bytes prove the fixed loop list, condition, quoting, and destination; exact report bytes prove the expected generated result. A parent or tutor must still require the oral mastery explanation before granting full mastery.
