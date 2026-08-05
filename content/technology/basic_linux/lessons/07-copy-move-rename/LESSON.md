# Lesson 07: Copy, move, and rename

## Objectives

By the end of this lesson, the student can:

- identify the source and destination operands in `cp SOURCE DESTINATION` and `mv SOURCE DESTINATION`;
- predict which source and destination paths will exist after a command;
- use `cp` to duplicate a file while preserving the original;
- use `mv` to relocate a file so its old path no longer exists;
- use `mv` to rename a file within one directory;
- recognize that copying, moving, and renaming preserve file contents in this lesson; and
- verify final paths and exact contents rather than relying on a command's silence.

## Standards

- **Primary — `PRIMER.DL.6.FILES.1`:** Create directories and move or copy files to organize a small workspace according to an explicit structure. This lesson directly assesses copying with `cp` and moving or renaming with `mv`.
- **Reinforcement — `PRIMER.DL.6.FILES.2`:** Inspect file contents with a safe read-only tool and confirm expected text after organizing files. The final task uses `cat` and deterministic content checks.

## Key ideas

A filesystem-changing command should follow a disciplined cycle: **inspect, predict, act, verify**. Before typing `cp` or `mv`, identify the existing source, resolve the intended destination, and state which paths should exist after success. Then run one precise command and inspect the result.

Both commands in this lesson take two path operands:

```text
COMMAND SOURCE DESTINATION
```

The **source** names the existing file being acted upon. The **destination** names where the resulting file should be. Operand order matters. Reversing the paths does not express the same request.

### Copying with `cp`

`cp` means copy. For a single file and a destination filename:

```console
$ cp inbox/checklist.txt archive/checklist.txt
```

Before:

```text
inbox/checklist.txt       exists
archive/checklist.txt     absent
```

After:

```text
inbox/checklist.txt       exists with the original content
archive/checklist.txt     exists with the same content
```

The source remains. A successful copy produces two independently named files. Later changes to one would not automatically change the other.

### Moving with `mv`

`mv` means move. It relocates the file's path:

```console
$ mv inbox/field-report.txt drafts/field-report.txt
```

Before:

```text
inbox/field-report.txt    exists
drafts/field-report.txt   absent
```

After:

```text
inbox/field-report.txt    absent
drafts/field-report.txt   exists with the report content
```

A move is not a copy followed by leaving both paths. The old source path no longer names the file.

### Renaming with `mv`

Linux uses the same `mv` command to rename a file. If the source and destination have the same parent directory but different final components, the file stays in that directory under a new name:

```console
$ mv drafts/field-report.txt drafts/ridge-report.txt
```

Only the name changes. The contents remain:

```text
drafts/field-report.txt   absent
drafts/ridge-report.txt   exists with the report content
```

“Move” and “rename” describe the intended result; `mv` handles both by changing the path that names the file.

### Destination caution

Basic `cp` and `mv` can replace an existing destination file. A command may do exactly what was typed even when that was not what the student intended. Therefore:

1. inspect the workspace;
2. confirm that source and destination are in the intended order;
3. predict the before-and-after paths;
4. run the smallest command; and
5. verify paths and contents.

The supplied activity is safe and deterministic: all requested destination paths begin absent, all work stays inside a fixture, and reset restores the original state. The caution still matters because the same commands affect real files outside practice.

## Vocabulary

- **source** — the existing path that a command reads or acts upon; the first path operand here.
- **destination** — the path where the command places or names the result; the second path operand here.
- **operand** — a value supplied to a command for it to act upon, such as a source or destination path.
- **copy** — create another file containing the same data while retaining the source file.
- **`cp`** — the command used here to copy one regular file to an exact destination path.
- **move** — relocate a file so the destination exists and the old source path no longer does.
- **rename** — change the name of a filesystem entry; performed here with `mv` inside one parent directory.
- **`mv`** — the command used to move or rename a filesystem entry.
- **parent directory** — the directory directly containing a file or directory.
- **final path component** — the name after the last `/` in a path.
- **overwrite** — replace the contents or entry already present at a destination path.
- **verification** — observe resulting paths and contents and compare them with the prediction.

## Worked prediction table

Require a prediction before every mutation. A useful form is:

| Operation | Source after success | Destination after success | Contents |
|---|---|---|---|
| `cp A B` | `A` exists | `B` exists | `A` and `B` match immediately after copying |
| `mv A B` to another directory | `A` absent | `B` exists | `B` has the former source content |
| `mv A B` in one directory | old name absent | new name exists | renamed file keeps its content |

This lesson avoids directory copies, options, wildcards, and destinations that are existing directories. Every destination includes its exact final filename so the state is unambiguous.

## Guided practice

The activity opens in the writable fixture directory `workspace/file-room`. Initially it contains:

```text
file-room/
├── BRIEF.txt
├── archive/
├── drafts/
├── inbox/
│   ├── checklist.txt
│   └── field-report.txt
└── published/
```

The intended final tree is:

```text
file-room/
├── BRIEF.txt
├── archive/
│   └── checklist.txt
├── drafts/
│   └── ridge-report.txt
├── inbox/
│   └── checklist.txt
└── published/
```

Complete one operation at a time.

1. **Copy:** Identify `inbox/checklist.txt` as the source and `archive/checklist.txt` as the destination. Predict that both paths will exist with `rope`, `water`, and `map` as their lines. Run the requested `cp` command.
2. **Move:** Identify `inbox/field-report.txt` as the source and `drafts/field-report.txt` as the destination. Predict that the inbox report path will disappear and the drafts path will contain the unchanged two-line report. Run the requested `mv` command.
3. **Rename:** Identify `drafts/field-report.txt` as the old path and `drafts/ridge-report.txt` as the new path. Because both have `drafts` as their parent, predict a rename rather than relocation to another directory. Run the requested `mv` command.
4. **Verify:** Read `archive/checklist.txt` and `drafts/ridge-report.txt` in that order with one `cat` command. Compare all five output lines with the expected content. Confirm that both checklist paths remain, only the renamed report path exists, and `published` remains unused.

Use the exact paths. Do not add options or create extra files. If a prediction and the observed state disagree, stop and reset instead of compounding the mistake.

## Expected content

The checklist must remain exactly:

```text
rope
water
map
```

The report must remain exactly:

```text
Ridge station clear.
Visibility: 12 km.
```

Path operations organize files; they do not edit this text.

## Mastery evidence

The terminal activity contains four ordered tasks with deterministic checks:

1. The student runs exact `cp inbox/checklist.txt archive/checklist.txt`. Checks prove that both paths are regular files and both contain the exact checklist text.
2. The student runs exact `mv inbox/field-report.txt drafts/field-report.txt`. Checks prove that the old path is absent, the new path is a file, its content is exact, and the earlier copy remains correct.
3. The student runs exact `mv drafts/field-report.txt drafts/ridge-report.txt`. Checks prove that the old draft name is absent, the new name exists, the report text is unchanged, and the cwd remains stable.
4. The student runs exact `cat archive/checklist.txt drafts/ridge-report.txt`. The output check proves the requested inspection order and exact combined output. Independent filesystem checks prove the final required paths, absent obsolete paths, preserved contents, unchanged brief, and unchanged working directory.

The command contract observes the latest command for each task, while filesystem and content checks establish state independently. The fixed fixture and exact checks make the result reproducible after reset.

The current contract cannot score a spoken prediction directly. A parent or tutor should require the student to state before each command: “The source is …; the destination is …; afterward these paths will exist …; these paths will be absent ….” Mastery requires accurate prediction, not merely reaching the final tree by trial and error.

## Parent or tutor review

Ask the student to explain the final tree without typing another command. Then ask:

1. Why does `inbox/checklist.txt` remain after the `cp` command?
2. Why does `inbox/field-report.txt` not remain after the first `mv` command?
3. How can the same command both move and rename a file?
4. Which path component changed during the rename?
5. Did moving or renaming edit either line of the report? What evidence supports the answer?
6. What could happen if a destination path already named an important file?
7. Why should source and destination be spoken in order before pressing Enter?
8. Why is a successful command with no output insufficient evidence of correct organization?

Require precise use of source, destination, parent directory, final component, copy, move, rename, content, and overwrite.

## Misconceptions

- **“`cp` removes the original.”** It leaves the source and creates a destination copy.
- **“`mv` leaves two files.”** A successful move leaves the destination path and removes the old source path.
- **“Renaming needs a command named `rename`.”** This lesson renames with `mv OLD-PATH NEW-PATH`.
- **“A rename changes the file's words.”** It changes the path that names the file, not the report content.
- **“The destination goes first.”** In these commands, the source is first and the destination is second.
- **“The shell can infer any intended filename.”** This lesson supplies exact destination filenames to remove ambiguity.
- **“The empty `published` directory is the obvious destination.”** Commands follow typed paths, not vague intent; only the specified destinations are correct.
- **“No output means the prediction was correct.”** Many successful coreutils commands are silent. Inspect the resulting state.
- **“An existing destination is always protected.”** Basic `cp` and `mv` may replace it, so inspect and predict first.
- **“Copying makes two permanently linked files.”** The copy initially has matching content but is an independent file.
- **“Moving to another directory and renaming are unrelated.”** Both change a path and are performed with `mv` here.

## Tutor strategy and constraints

Use graduated coaching, not immediate answers:

1. First ask the student to identify source, destination, and predicted paths after success.
2. If needed, name the appropriate command and ask the student to construct the operands.
3. Only at the final hint level provide the exact single command for the current task.

Keep coaching within `cp`, `mv`, relative source/destination paths, and final read-only inspection with `cat`. Do not introduce `rm`, `mkdir`, `touch`, redirection, editors, wildcards, recursion, force or interactive options, shell chaining, or absolute paths. Do not provide all four commands in one pasteable block. If the state becomes incorrect, direct the student to reset, reconstruct the prediction, and try again.

Do not accept a superficially similar result. An extra report copy is not equivalent to a move. A report in `published` is not equivalent to the requested draft location. A renamed file with altered content is not correct. Precision in both path state and content is the evidence.

## Safety and craftsmanship

- Work only inside `workspace/file-room`.
- Fixture directories use mode `0755`, so `inbox`, `archive`, `drafts`, and `published` are writable and traversable.
- Fixture files use mode `0644`; the exercise does not depend on privileged access or read-only source files.
- Requested destination paths begin absent, preventing an intended exercise step from overwriting fixture data.
- Use exact relative paths and exact names; Linux names are case-sensitive.
- Do not use options in this introductory exercise. Directory copying and overwrite-control options belong in later guided work.
- Do not place anything in `published`; its presence tests whether the student follows exact destinations rather than guessing from directory names.
- If anything unexpected occurs, stop. Read the task, reset the fixture, and restore the inspect-predict-act-verify cycle.

## Extension

After mastery, use a verbal or paper exercise rather than changing the assessed fixture. For each command below, draw a before-and-after tree and identify the source and destination:

```text
cp notes/day-1.txt backup/day-1.txt
mv notes/day-2.txt review/day-2.txt
mv review/day-2.txt review/day-02.txt
```

For each, answer:

- Does the old source path remain?
- Does the destination path exist afterward?
- Is the operation best described as copy, move, or rename?
- Which contents should be present at the destination?
- What should be checked before running the command on real files?

As a craftsmanship discussion, ask what additional caution would be needed if a destination already existed. The answer should emphasize inspection and avoiding accidental replacement; option syntax is intentionally outside this lesson's assessed scope.
