# Lesson 12: Search inside files

## Objectives

By the end of this lesson, the student can:

- distinguish searching **inside files** with `grep` from searching for path names with `find`;
- use `grep 'LITERAL TEXT' FILE` to print complete lines containing a literal phrase;
- treat capitalization as significant in ordinary `grep` searches;
- use the simple pattern anchor `^` to require a match at the beginning of a line;
- add `-n` to report source line numbers;
- search several explicitly named files with one pattern;
- interpret one-file output and `filename:line-number:line` multiple-file output;
- choose exact relative file paths as the search scope; and
- keep content searches read-only and all fixtures unchanged.

## Standards

- **Primary — `PRIMER.DL.6.FILES.2`:** Inspect file contents with safe read-only tools and confirm expected text. The student uses `grep` to select exact matching lines from controlled logs and documents, while preservation checks prove that inspection did not change source files.
- **Reinforcement — `PRIMER.DL.6.NAV.2`:** Distinguish files from directories and follow relative paths inside a workspace. The student searches explicit paths under `logs/` and `docs/`, interprets filename prefixes, and excludes matching text in files outside the named scope.

## The question determines the tool

The previous lesson used `find` to locate filesystem paths by name or type. This lesson asks a different question:

- **Where is a file whose name is `system.log`?** Search paths with `find`.
- **Which lines inside `system.log` begin with `ERROR`?** Search text with `grep`.

`grep` reads text and prints lines that match a pattern. In this lesson it does not recurse through directories and it does not discover files. Every file to inspect is named explicitly.

The general form is:

```text
grep OPTIONS 'PATTERN' FILE...
```

Read it from left to right:

1. `grep` is the text-search program.
2. An option such as `-n` changes the report.
3. The quoted pattern describes text a line must contain.
4. One or more file operands define exactly which files are read.

## Literal text searches

The first task uses:

```console
$ grep 'calibration complete' logs/sensor.log
08:07 calibration complete
08:30 calibration complete
```

The pattern is the adjacent text `calibration complete`. The space is part of the pattern. Quotes keep the two words together as one shell argument. The quotes are shell syntax and are removed before `grep` receives the pattern.

For introductory literal searches:

- spell the text exactly;
- preserve capitalization;
- quote the whole pattern;
- name the exact file; and
- predict matching and nonmatching lines before running the command.

`grep` prints each **complete matching line**. It does not normally print only the words `calibration complete`, and it does not print nonmatching lines surrounding them.

### Literal does not mean whole line

The literal phrase may occur anywhere in a line. In:

```text
08:07 calibration complete
```

`calibration complete` begins after the timestamp, yet the line matches because the phrase occurs within it.

The lesson uses plain text such as `timeout` as a literal. Some characters have special meanings in patterns; only the beginning anchor `^` is introduced here. More advanced pattern syntax belongs later.

## Case sensitivity

Ordinary `grep` matching is case-sensitive:

- `ERROR` matches uppercase `ERROR`;
- `ERROR` does not match lowercase `error`;
- `TODO:` matches uppercase `TODO:`;
- `TODO:` does not silently broaden to other capitalization.

This precision is useful when logs use capitalization as a record marker. Do not add a case-insensitive option unless the question explicitly calls for one; this lesson never does.

## A simple pattern: beginning of line

The caret `^` at the start of a pattern means **the beginning of a line**. It is an anchor, not a character that must appear in the file.

```console
$ grep -n '^ERROR' logs/system.log
2:ERROR fan stalled
4:ERROR backup failed
```

The pattern `^ERROR` means “uppercase `ERROR` starting at position one.” It excludes:

- `error lowercase decoy` because capitalization differs; and
- a line such as `NOTICE: ERROR was cleared` because `ERROR` would not begin that line.

Likewise, `^TODO:` selects a task marker only when it starts the record. It excludes sentence-internal text such as:

```text
A TODO: inside a sentence is not a task marker.
```

Only `^` is taught as pattern syntax here. Do not infer meanings for other punctuation yet.

## Line numbers with `-n`

Use `-n` when the location of each match matters:

```console
$ grep -n '^ERROR' logs/system.log
2:ERROR fan stalled
4:ERROR backup failed
```

For one named file, each output record is:

```text
line-number:matching-line
```

Thus `2:ERROR fan stalled` means that the complete matching text came from line 2 of `logs/system.log`.

`-n` does **not** mean:

- print only line *n*;
- stop after *n* matches; or
- number the matches 1, 2, 3 regardless of their source positions.

It reports the original source line number.

## Searching multiple files

One `grep` command can apply the same pattern to several explicitly named files:

```console
$ grep -n 'timeout' logs/app.log logs/network.log
logs/app.log:2:09:04 request timeout after 30s
logs/network.log:2:09:03 gateway timeout
logs/network.log:4:09:18 timeout counter reset
```

When multiple files are searched with `-n`, read each record as:

```text
filename:line-number:matching-line
```

The prefix identifies both the source file and source line. The file operand order also controls the grouped output order in this deterministic activity: matches from `logs/app.log` are reported before matches from `logs/network.log` because that is the operand order.

A third file, `logs/archive.log`, also contains `timeout`, but it is not named in the command. Therefore it is outside the search scope and contributes no output. `grep` does not automatically inspect sibling files merely because they have similar names or extensions.

## Exact path scope

Every task names one or two regular files. Use those paths exactly:

```text
logs/sensor.log
logs/system.log
logs/app.log
logs/network.log
docs/field-guide.txt
docs/maintenance.txt
```

The directory component matters. From `workspace/grep-lab`, `logs/app.log` means the file `app.log` inside the child directory `logs`. A bare `app.log` would refer to a different location and fail here.

Explicit operands make intent auditable:

- a reader knows which files may be inspected;
- matching decoys outside the scope stay excluded;
- output prefixes can be interpreted against known paths; and
- accidental broad searches are avoided.

Do not replace explicit paths with `*`, directory operands, or recursive options in this lesson.

## Exit status and empty results

These tasks all have known matches and therefore expect exit status 0. In general, `grep` uses:

- **0** — at least one selected line matched;
- **1** — no line matched; and
- **greater than 1** — an error occurred, such as an unreadable or missing operand.

No output with status 1 can be a correct finding: no line in the named files matched the exact pattern. It does not mean the source should be edited to manufacture a match. First recheck the requested scope, spelling, capitalization, anchor, and path.

## Safe read-only searching

The four taught forms only read files and print selected lines. They do not edit source content. Keep them read-only:

```text
grep 'calibration complete' logs/sensor.log
grep -n '^ERROR' logs/system.log
grep -n 'timeout' logs/app.log logs/network.log
grep -n '^TODO:' docs/field-guide.txt docs/maintenance.txt
```

Do not add:

- `>` or `>>` output redirection;
- `tee`;
- a pipe to an editing or removal command;
- command substitution;
- `sed`, `awk`, or another transformer;
- recursive options `-r` or `-R`;
- wildcard file operands; or
- any command that creates, copies, renames, edits, or removes fixture files.

The files are mode `0444` to signal read-only source material. If any fixture changes, reset the activity rather than trying to repair it with more commands.

## Vocabulary

- **`grep`** — a program that reads text and prints lines selected by a pattern.
- **pattern** — the text rule used to decide whether a line matches.
- **literal text** — ordinary characters intended to match themselves, such as `timeout` or `calibration complete` in these tasks.
- **match** — a line that satisfies the pattern.
- **matching line** — the complete source line printed by `grep`.
- **case-sensitive** — treating uppercase and lowercase letters as different.
- **anchor** — pattern syntax requiring a position rather than matching a visible character.
- **`^`** — the beginning-of-line anchor when it appears at the start of these patterns.
- **`-n`** — an option that adds each matching line's source line number.
- **file operand** — an explicitly named file for `grep` to read.
- **scope** — the files included in a search; here scope is defined by exact file operands.
- **output prefix** — source information printed before a matching line, such as a filename and line number.
- **exit status** — the numeric result indicating match, no match, or error.
- **fixture** — supplied resettable content with known matches and deliberate decoys.

## Fixture map

The activity opens in `workspace/grep-lab`:

```text
grep-lab/
├── docs/
│   ├── field-guide.txt
│   ├── maintenance.txt
│   └── readme.txt
└── logs/
    ├── app.log
    ├── archive.log
    ├── network.log
    ├── sensor.log
    └── system.log
```

The fixture is designed to make scope and patterns observable:

- `sensor.log` contains two lines with the exact phrase `calibration complete`;
- `system.log` contains uppercase line-start `ERROR` records and a lowercase decoy;
- `app.log` and `network.log` contain three in-scope `timeout` lines total;
- `archive.log` contains an out-of-scope `timeout` line;
- the two assigned documents contain four line-start `TODO:` records;
- those documents also contain sentence-internal `TODO:` decoys; and
- `readme.txt` contains an out-of-scope line-start `TODO:` decoy.

All files are read-only mode `0444`; directories are `0755`.

## Guided practice

Complete four tasks in order. Before each command, say:

> “My pattern is ___. Capitalization is ___. I will search only ___. I expect the output fields ___, and ___ is a deliberate nonmatch or out-of-scope file.”

### 1. Literal phrase in one log

Question: Which lines in `logs/sensor.log` contain the adjacent phrase `calibration complete`?

```console
$ grep 'calibration complete' logs/sensor.log
08:07 calibration complete
08:30 calibration complete
```

Interpretation:

- the quoted phrase is one pattern argument;
- both complete matching lines are printed;
- `08:05 calibration started` does not contain the exact phrase; and
- no filename is prefixed because only one file was searched and no option requested a filename.

### 2. Numbered line-start errors

Question: Which lines in `logs/system.log` begin with uppercase `ERROR`, and where are they?

```console
$ grep -n '^ERROR' logs/system.log
2:ERROR fan stalled
4:ERROR backup failed
```

Interpretation:

- `^` requires `ERROR` at the beginning;
- uppercase and lowercase differ;
- `-n` reports original source lines 2 and 4; and
- with one file, output begins with line number rather than filename.

### 3. One literal across two logs

Question: Which lines in exactly `logs/app.log` and `logs/network.log` contain `timeout`, and where are they?

```console
$ grep -n 'timeout' logs/app.log logs/network.log
logs/app.log:2:09:04 request timeout after 30s
logs/network.log:2:09:03 gateway timeout
logs/network.log:4:09:18 timeout counter reset
```

Interpretation:

- the same literal pattern is applied to both operands;
- filename and line number identify each source;
- output follows the requested file order; and
- `logs/archive.log` is not searched, despite containing the literal.

### 4. Anchored markers across two documents

Question: Which lines in exactly the two assigned documents begin with `TODO:`, and where are they?

```console
$ grep -n '^TODO:' docs/field-guide.txt docs/maintenance.txt
docs/field-guide.txt:2:TODO: label spare batteries
docs/field-guide.txt:5:TODO: pack the compass
docs/maintenance.txt:3:TODO: replace the filter
docs/maintenance.txt:5:TODO: test the alarm
```

Interpretation:

- the anchor excludes `TODO:` occurring later in a sentence;
- filename and line number identify all four source locations;
- `docs/readme.txt` is out of scope because it is not an operand; and
- all source files remain unchanged.

## Mastery evidence

The activity uses four ordered tasks with deterministic command, output, path, and preservation evidence:

1. Exact observation of `grep 'calibration complete' logs/sensor.log`, exact two-line output, cwd, and exact source content prove a one-file literal phrase search.
2. Exact observation of `grep -n '^ERROR' logs/system.log`, exact numbered output, cwd, and preserved source prove beginning-anchor use, case sensitivity, and line-number interpretation.
3. Exact observation of `grep -n 'timeout' logs/app.log logs/network.log`, exact filename-and-line-number output, and preservation of both searched logs plus the out-of-scope archive prove explicit multiple-file scope.
4. Exact observation of `grep -n '^TODO:' docs/field-guide.txt docs/maintenance.txt`, exact four-line output, preservation of both sources and the out-of-scope readme, and final cwd prove the combined skill.

`command_properties` observes the executable and arguments after shell parsing. It can prove that a phrase arrived as one argument and that `^` arrived literally in the pattern, but quote characters themselves are removed by the shell. Task directions and tutor review therefore require the quoted form even though command observation cannot distinguish every equivalent quoting style.

`pipeline_output` observes the latest standard output and does not by itself prove which command produced it. Pairing it with exact latest-command evidence and immutable fixture checks makes each task deterministic within the current contract. Source preservation checks independently prove that the expected files still contain their original text.

The verifier cannot directly grade the student's explanation of why a line, capitalization variant, or sibling file was excluded. A parent or tutor must require precise oral interpretation before granting conceptual mastery.

## Parent or tutor review

Ask the student to answer without running more commands:

1. What question does `grep` answer that `find` does not?
2. In `grep OPTIONS 'PATTERN' FILE...`, what job does each part perform?
3. Why is `'calibration complete'` quoted?
4. Does `grep` print only the matching words or the complete matching line here?
5. Is ordinary `grep` matching case-sensitive?
6. Why does uppercase `ERROR` not match lowercase `error`?
7. What does `^` mean at the beginning of `'^ERROR'`?
8. Does `^` represent a visible caret in the source file?
9. Why would sentence-internal `TODO:` fail `'^TODO:'`?
10. What information does `-n` add?
11. Does `-n` limit the number of matches?
12. How do you read `logs/network.log:4:09:18 timeout counter reset`?
13. Why does one-file numbered output omit a filename in this activity?
14. Why does multiple-file output include filenames?
15. Why is `logs/archive.log` absent from the timeout result?
16. Why is `docs/readme.txt` absent from the TODO result?
17. What does exit status 1 normally mean for `grep`?
18. Which checks show that all searched and decoy files remained unchanged?
19. Why are recursive options and wildcard file operands excluded here?
20. What should you inspect first if expected output is empty?

Require the student to refer to exact pattern text, capitalization, line position, and file scope rather than merely reciting commands.

## Misconceptions

- **“`grep` finds filenames.”** `grep` searches text records. `find` searches filesystem paths.
- **“`grep` prints only the matched word.”** In these forms it prints the complete source line.
- **“Spaces separate patterns.”** Unquoted spaces separate shell arguments. Quote a multiword pattern so it remains one argument.
- **“Capitalization does not matter.”** Plain `grep` is case-sensitive.
- **“`^` means the source must contain a caret.”** At the start of these patterns, it anchors the match to the beginning of a line.
- **“`^TODO:` means any line containing `TODO:`.”** It means lines beginning with exactly uppercase `TODO:`.
- **“`-n` asks for n lines.”** It adds original source line numbers.
- **“The number in output counts matches.”** It identifies the line's position in the source file.
- **“Multiple-file output is ambiguous.”** `grep` prefixes each match with its source filename; `-n` then adds line number.
- **“A matching sibling file is searched automatically.”** Only named operands are searched in this lesson.
- **“A directory operand means search all files below it.”** Recursive directory searching requires other behavior and is intentionally excluded.
- **“No output always means an error.”** Status 1 can correctly mean no line matched the exact pattern in the named files.
- **“Read-only commands require no scope discipline.”** Exact paths prevent irrelevant or unintended inspection even when data is not modified.

## Tutor strategy and constraints

For every task, use this graduated sequence:

1. Ask the student to name the exact file or files permitted by the question.
2. Ask for the exact capitalization and literal text.
3. Ask whether the text may occur anywhere or must begin the line.
4. Ask whether source line numbers are needed.
5. Ask what prefixes one-file or multiple-file output should contain.
6. Require predictions naming expected matches and at least one decoy.
7. Reveal the exact command only at hint level 3.
8. After execution, require interpretation of every output field.

Keep coaching strictly within:

```text
grep 'calibration complete' logs/sensor.log
grep -n '^ERROR' logs/system.log
grep -n 'timeout' logs/app.log logs/network.log
grep -n '^TODO:' docs/field-guide.txt docs/maintenance.txt
```

Do not introduce recursive options, case folding, inverted matches, counts, filename-only output, match-only output, extended patterns, fixed-string flags, context lines, standard input, wildcard operands, directory operands, dot/star syntax, bracket expressions, alternation, grouping, repetition, pipes, redirection, `tee`, command substitution, loops, aliases, or command chaining. Do not substitute `cat`, `find`, `sed`, or `awk` for the requested evidence.

If output differs, inspect in this order:

1. current working directory;
2. exact relative file operands and their order;
3. option spelling and placement;
4. whether the pattern remained one argument;
5. capitalization;
6. presence or absence of `^`; and
7. fixture integrity.

Reset if any fixture changed.

## Safety and craftsmanship

- Remain in `workspace/grep-lab`.
- Name only the requested files.
- Quote every lesson pattern.
- Preserve exact capitalization.
- Use `^` only when the question says “begins with.”
- Use `-n` only when source locations are requested.
- Predict output structure before running the command.
- Read every filename, line number, and matching line.
- Keep all source files byte-for-byte unchanged.
- Prefer a precise bounded search over a broad convenient one.
