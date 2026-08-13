# Lesson 17: Variables and arguments

## Objectives

By the end of this lesson, the student can:

- write a Bash assignment as `name=value` with no spaces around `=`;
- quote a value containing spaces and explain that quotes are syntax, not stored data;
- expand a variable as `"$name"` so its value remains one argument;
- explain `$1`, `$2`, and the all-arguments concept `"$@"`;
- use `printf` with fixed formats for stable output;
- build and inspect a bounded script, add only user execute permission, run it with arguments, and verify its content, mode, and generated report.

## Standards

Scripting-specific digital-literacy standards are pending, so this lesson provisionally uses the closest current file standards.

- **Primary — `PRIMER.DL.6.FILES.1`:** The student maintains a bounded script and creates one report at explicit relative paths.
- **Reinforcement — `PRIMER.DL.6.FILES.2`:** The student inspects exact script text, permission mode, and generated output.

The activity records `standardsStatus: pending-scripting-standards` explicitly.

## Core ideas

### Assignment syntax

A Bash assignment is one shell word:

```bash
label='Field report'
```

There are no spaces around `=`. The quotes allow the value to contain a space; they are removed by the shell and are not stored in the value.

These are different:

```bash
label='Field report'   # assignment
label = 'Field report' # parsed as a command named label, not an assignment
```

### Quote expansions

The usual safe form is:

```bash
printf 'Title: %s\n' "$label"
```

Double quotes still allow `$label` to expand, but preserve its result as one argument. Without quotes, spaces can split a value into several words and wildcard characters can trigger filename matching. In this lesson, values are data and must retain their boundaries.

### Positional arguments

Given this invocation:

```bash
./argument-report.sh 'north ridge' 'kit box' map
```

Bash removes the invocation quotes and supplies three arguments:

| Expansion | Value |
|---|---|
| `"$1"` | `north ridge` |
| `"$2"` | `kit box` |
| `"$@"` | all three arguments, each preserved as a separate word |

The quote characters are shell syntax, not part of the values. `$1` and `$2` select positions. `"$@"` is special: it represents every positional argument while retaining the boundary of each one.

### Stable output with `printf`

Use a fixed format and pass changing values as data:

```bash
printf 'First: %s\n' "$1"
```

`%s` receives one value and `\n` emits a newline. This is more explicit and predictable than relying on differing `echo` behaviors.

Bash `printf` reuses a format when more data arguments remain:

```bash
printf ' <%s>' "$@"
```

With the three lesson arguments, this emits:

```text
 <north ridge> <kit box> <map>
```

This demonstrates `"$@"` without introducing a loop; loops belong to lesson 18.

## The bounded script

The completed `argument-report.sh` is exactly:

```bash
#!/bin/bash
# Report a label and positional arguments without changing them.
label='Field report'
mkdir -p output
printf 'Title: %s\nFirst: %s\nSecond: %s\nAll:' "$label" "$1" "$2" > output/argument-report.txt
printf ' <%s>' "$@" >> output/argument-report.txt
printf '\n' >> output/argument-report.txt
```

Line by line:

1. The shebang selects Bash for direct execution.
2. The comment states the script's purpose.
3. `label='Field report'` assigns a two-word value with valid syntax.
4. `mkdir -p output` ensures the bounded destination directory exists.
5. The first `printf` uses quoted expansions and `>` to start a fresh report.
6. The next `printf` receives every argument separately through `"$@"` and appends one bracketed field per argument.
7. The final `printf` appends an explicit newline.

The first write uses `>`, so each run replaces any old report before the two `>>` operations finish that run. Repeating the same invocation therefore produces the same final bytes rather than accumulating copies.

## Activity sequence

The fixture begins as:

```text
workspace/argument-lab/
└── README.txt            (read-only; preserve)
```

Complete five tasks in order:

1. **Practice assignment and quoting.** Run the supplied `bash -c` experiment and confirm `Place: north ridge`.
2. **Edit and inspect.** Create only the assigned script with the exact seven lines, then use `cat` to read it. The output report must not exist yet.
3. **Add permission.** Run `chmod u+x argument-report.sh`; verify mode `0744` and unchanged text.
4. **Run with arguments.** Execute `./argument-report.sh 'north ridge' 'kit box' map`.
5. **Verify.** Read the generated report and account for every value and newline.

The exact generated report is:

```text
Title: Field report
First: north ridge
Second: kit box
All: <north ridge> <kit box> <map>
```

The final tree is:

```text
workspace/argument-lab/
├── README.txt
├── argument-report.sh    (mode 0744)
└── output/
    └── argument-report.txt
```

## Mastery check

Require the student to explain, not merely reproduce, these facts:

1. Why must an assignment omit spaces around `=`?
2. Why are the quotes around `Field report` not stored in the value?
3. What problem does `"$label"` prevent?
4. How do invocation quotes make `north ridge` one argument?
5. What do `$1` and `$2` select?
6. How does `"$@"` differ from one combined string?
7. Why does `printf ' <%s>' "$@"` produce three fields without a loop?
8. Why do editing and `chmod` not create the report?
9. Why is the final mode exactly `0744`?
10. Why does another identical run leave identical report content?

## Tutor boundaries

Keep instruction within assignment syntax, basic quoting, `$1`, `$2`, the `"$@"` concept, and fixed-format `printf`. Do not introduce arrays, `eval`, environment configuration, `$*`, `$#`, `shift`, functions, conditions, loops, arithmetic, command substitution, or advanced parameter expansion. Do not substitute `echo`, broaden permissions, change paths, or permit unrelated files.

Give graduated hints in order. Before each command, ask the student to predict whether script content, permission mode, or generated output can change. Reset after changes outside `workspace/argument-lab`, altered required script text, broad permissions, different run arguments, or edits to `README.txt`.

## Verification and contract limits

The activity combines exact content checks, mode checks, path type checks, immutable-fixture checks, current-directory checks, latest-command evidence, and exact output checks. These prove the bounded final state and required inspect/run commands.

The v1 contract observes only the latest command and cannot directly score the student's explanation or prove every shell expansion inside an executed script. The exact script bytes plus exact generated bytes are strong deterministic evidence, but a parent or tutor must still require the oral explanation above before granting mastery.
