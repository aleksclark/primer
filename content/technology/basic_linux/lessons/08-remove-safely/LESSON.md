# Lesson 08: Remove safely

## Objectives

By the end of this lesson, the student can:

- explain that ordinary command-line removal does not place an item in a trash folder and should be treated as irreversible;
- inspect a small assigned target before removing anything;
- use `rm PATH` to remove one precisely named file;
- use `rmdir PATH` to remove one precisely named empty directory;
- explain why `rmdir` refuses a non-empty directory;
- use `rm -r PATH` only for one explicitly named, inspected, disposable directory tree inside the lesson sandbox; and
- verify that intended targets are absent while unrelated files remain.

## Standards

- **Primary — `PRIMER.DL.6.FILES.1`:** Organize a small workspace according to an explicit structure. This lesson extends careful file organization to removal of obsolete entries, with exact final-state checks.
- **Reinforcement — `PRIMER.DL.6.NAV.2`:** Distinguish files from directories and follow precise relative paths. The student must identify each target's type and resolve its path before removal.

## Key ideas

Removal requires more restraint than creation, copying, or moving. A mistyped creation command usually leaves an extra item that can be corrected. A successful removal command destroys a directory entry and may destroy its data. In this lesson there is no trash folder and no undo command. Treat removal as **irreversible**.

Use a disciplined cycle every time:

1. **Locate** — confirm the current working directory and stay inside the assigned sandbox.
2. **List** — inspect the exact parent or directory tree that contains the target.
3. **Name** — say the exact target path and whether it is a file, an empty directory, or a non-empty directory.
4. **Remove** — run the smallest command that fits that one target.
5. **Verify** — list again and confirm both what disappeared and what remained.

This is the **preflight** habit: inspection and prediction happen before pressing Enter. Read the complete path from left to right. Do not rely on what you remember seeing earlier, and do not act on a vague instruction such as “clear this area.”

### Remove one file with `rm`

For one known file, give `rm` its exact relative path:

```console
$ rm cleanup-zone/stale-note.txt
```

After successful removal, that path no longer exists. The command does not move the note to a recoverable trash folder. `rm` is not needed for a directory in this part of the lesson.

### Remove one empty directory with `rmdir`

`rmdir` removes a directory only when it is empty:

```console
$ rmdir cleanup-zone/empty-bin
```

This limitation is useful protection. If the directory contains an entry, `rmdir` refuses rather than deleting the contents. Stop and inspect when a command refuses; do not automatically reach for a more powerful command.

### Limited recursive removal with `rm -r`

A directory containing files is a tree. Recursive removal visits the named directory and its descendants. That power makes the target path especially important.

The only recursive removal practiced here is:

```console
$ rm -r cleanup-zone/old-bundle
```

It is acceptable here only because all of these statements are true:

- `old-bundle` was supplied as disposable practice data;
- the student listed `cleanup-zone` first and saw the complete small tree;
- the path is a precise relative path inside `workspace/removal-lab`;
- the task explicitly names the whole directory for removal; and
- reset can rebuild the fixture.

This lesson does **not** teach broad deletion. Never substitute a wildcard, the current directory, a parent directory, an absolute system path, or a vaguely chosen larger directory. Do not add force options. Recursive removal should be exceptional, bounded, inspected, and explicit.

## Vocabulary

- **remove** — make a filesystem entry cease to exist at its path.
- **irreversible** — not automatically recoverable by an undo operation or trash folder in this terminal activity.
- **preflight** — inspect location, target, type, and expected effect before a destructive command.
- **target** — the exact path a command will act upon.
- **`rm`** — the command used here to remove one precisely named regular file, or with `-r`, one precisely named disposable directory tree.
- **`rmdir`** — a command that removes an empty directory and refuses a non-empty one.
- **empty directory** — a directory containing no entries other than the implicit filesystem references.
- **recursive** — operating on a directory and its descendants.
- **descendant** — an entry nested below a directory.
- **precise path** — a path that identifies exactly the intended target without a wildcard or ambiguous scope.
- **verify** — inspect the resulting state and compare it with the prediction.
- **sandbox** — the isolated practice workspace whose fixture can be reset.

## Command choice

| Target known from preflight | Smallest command in this lesson | Expected result |
|---|---|---|
| One regular file | `rm EXACT-FILE-PATH` | That file path is absent |
| One empty directory | `rmdir EXACT-DIRECTORY-PATH` | That directory path is absent |
| One inspected disposable non-empty tree | `rm -r EXACT-DIRECTORY-PATH` | That directory and its descendants are absent |

Do not choose by convenience. Choose by target type and required scope. In particular, a refusal from `rmdir` is information: the directory is not empty. Inspect it and reconsider the request rather than escalating automatically.

## Guided practice

The activity opens in `workspace/removal-lab`. Its initial fixture is:

```text
removal-lab/
├── BRIEF.txt
├── cleanup-zone/
│   ├── empty-bin/
│   ├── old-bundle/
│   │   ├── draft.txt
│   │   └── notes/
│   │       └── obsolete.txt
│   └── stale-note.txt
└── keep/
    └── current.txt
```

Only the three explicitly named targets under `cleanup-zone` are disposable:

- `cleanup-zone/stale-note.txt`
- `cleanup-zone/empty-bin`
- `cleanup-zone/old-bundle`

`BRIEF.txt`, `keep`, and `keep/current.txt` must remain unchanged.

Complete the five tasks in order:

1. **Preflight the cleanup zone.** Run `ls -R cleanup-zone`. Identify `stale-note.txt` as a file, `empty-bin` as an empty directory, and `old-bundle` as a non-empty directory containing two descendants at different levels. Say which removal command fits each target before changing anything.
2. **Remove one file.** Run `rm cleanup-zone/stale-note.txt`. Predict that only this file path will disappear at this step.
3. **Remove one empty directory.** Run `rmdir cleanup-zone/empty-bin`. Its emptiness was established during preflight. Do not use recursive removal for an empty directory.
4. **Remove one bounded disposable tree.** Re-read the exact path, then run only `rm -r cleanup-zone/old-bundle`. The whole supplied tree is disposable; no broader path is authorized.
5. **Verify the result.** Run `ls -R .`. Confirm that `cleanup-zone` remains but is empty, while `BRIEF.txt`, `keep`, and `keep/current.txt` remain. Confirm that all three intended targets are absent.

The final tree is:

```text
removal-lab/
├── BRIEF.txt
├── cleanup-zone/
└── keep/
    └── current.txt
```

If anything differs from the preflight prediction, stop and reset. Do not improvise additional removal commands.

## Mastery evidence

The terminal activity contains five ordered tasks with deterministic command and filesystem checks:

1. The student runs exact `ls -R cleanup-zone`; output checks establish that the preflight showed every named target and both descendants of `old-bundle`.
2. The student runs exact `rm cleanup-zone/stale-note.txt`; checks establish that the file is absent while the two directory targets and protected content remain.
3. The student runs exact `rmdir cleanup-zone/empty-bin`; checks establish that the empty directory is absent while the non-empty bundle remains.
4. The student runs exact `rm -r cleanup-zone/old-bundle`; checks establish that the bundle and its descendants are absent while the parent cleanup zone and protected content remain.
5. The student runs exact `ls -R .`; final checks establish the three target paths are absent, `cleanup-zone` and `keep` still have the correct types, protected contents are exact, and the working directory did not change.

Exact command checks prevent a broad or differently scoped command from satisfying a task merely because it happened to produce a similar final state. Independent filesystem checks prove preservation as well as deletion. The fixture is disposable and resettable, but the habit being assessed is suitable for real work: preflight, exact scope, smallest command, verification.

The current contract cannot score the student's spoken prediction. A parent or tutor should require this statement before each removal: “My exact target is …; it is a file/empty directory/non-empty directory; I will use …; afterward … will be absent and … will remain.”

## Parent or tutor review

Ask the student to explain, without typing another command:

1. Why should command-line removal be treated as irreversible?
2. What did the preflight listing prove about each target's type and contents?
3. Why was `rm` appropriate for `stale-note.txt`?
4. Why was `rmdir` better than recursive removal for `empty-bin`?
5. What useful warning would `rmdir` provide if a directory were not empty?
6. Why was recursive removal allowed for `old-bundle` but not for `cleanup-zone`?
7. Which exact evidence proves `keep/current.txt` survived unchanged?
8. Why is a target's parent path just as important as its final name?
9. What should happen if the typed path does not exactly match the path that was inspected?
10. Why should a student stop rather than increase a command's power after an unexpected refusal?

Require precise use of target, relative path, empty, recursive, descendant, preflight, verify, and irreversible.

## Misconceptions

- **“Removed files go to trash.”** These commands do not provide a trash or undo step in the activity.
- **“`rm` and `rmdir` are interchangeable.”** Plain `rm` removes the named file here; `rmdir` removes only an empty directory.
- **“`rmdir` is broken when it refuses a directory.”** Refusal usually means the directory is not empty; that is protective information.
- **“Recursive means only the directory itself.”** It includes every descendant below the named tree.
- **“A short target is always safer.”** A short vague path can name a much larger scope than intended. Safety comes from exact inspection and exact scope.
- **“If one recursive target is safe, a broader target is also safe.”** Authorization applies only to the inspected, explicitly named disposable tree.
- **“The sandbox makes careless syntax acceptable.”** The data is disposable so practice can be reset; the required habit is still precision.
- **“No output proves the correct item was removed.”** Successful removal is often silent. Verify both absence and preservation.
- **“After an error, add more power until the command works.”** Stop, inspect, and understand the refusal first.
- **“Only the final filename matters.”** Every path component determines the actual target.

## Tutor strategy and constraints

Use graduated coaching for each task:

1. Ask the student to identify the cwd, exact target, target type, and expected preserved paths.
2. Ask which of the three narrowly taught forms fits that type.
3. Only at the final hint level provide the exact command for the current task.

Keep coaching within read-only listing, exact relative paths, plain `rm` for the named file, `rmdir` for the named empty directory, and `rm -r` solely for `cleanup-zone/old-bundle`. Never suggest wildcards, current- or parent-directory targets, absolute paths, force options, multiple deletion operands, shell expansion, command chaining, or a broader recursive target. Do not provide all removal commands as one pasteable block.

If the student mistypes, acts outside the requested order, removes protected content, or reaches an unexpected state, direct a fixture reset. Do not teach recovery tools, aliases, interactive or force options, or alternative deletion programs in this lesson. A tutor must not praise a broad command merely because the final visible tree looks right.

## Safety and craftsmanship

- Work only inside `workspace/removal-lab`.
- The activity fixture is disposable and resettable; real files may not be.
- List before removal and list after removal.
- Use exactly one target operand in each removal task.
- Use relative paths beginning with `cleanup-zone/` for every removal.
- Preserve `cleanup-zone` itself, `BRIEF.txt`, `keep`, and `keep/current.txt`.
- Use recursive removal only once and only for `cleanup-zone/old-bundle`.
- Do not add force or convenience options.
- Do not use wildcards or broad directory targets.
- Stop when a command refuses or when the observed state differs from the prediction.
- Precision includes what remains, not only what disappears.

## Extension

Use a verbal or paper exercise only; do not run more deletion commands in the assessed fixture. For each case below, choose the smallest command form and explain the required preflight:

- a precisely named obsolete regular file;
- a precisely named empty staging directory;
- a supplied disposable directory containing two known files;
- a directory whose contents have not yet been inspected;
- a request that says only “remove old things.”

The final two cases should not lead directly to removal. First inspect the unknown directory; reject or clarify the vague request. For every case that is safe to proceed, state the exact target, its type, its known contents, what will remain, and how the result will be verified.
