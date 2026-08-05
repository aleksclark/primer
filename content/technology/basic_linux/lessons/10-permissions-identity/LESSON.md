# Lesson 10: Permissions and identity

## Objectives

By the end of this lesson, the student can:

- explain that every running process acts with an operating-system user identity and group memberships;
- distinguish that operating-system identity concept from a student's Primer profile or typed name;
- read the path type, user, group, and other permission fields in `ls -l` output;
- interpret `r`, `w`, `x`, and `-` for regular files;
- explain why directory `x` means traversal/search rather than “execute the directory”;
- predict how removing directory execute affects access to entries whose names are known;
- use narrow symbolic changes with `chmod u+w`, `chmod g+w`, `chmod a-x`, and `chmod u+x`; and
- preserve source fixtures by changing permissions only on disposable copies.

## Standards

- **Primary — `PRIMER.DL.6.FILES.1`:** Create directories and move or copy files to organize a small workspace according to an explicit structure. The student makes clearly named disposable copies before changing permissions and preserves the original organization and source material.
- **Reinforcement — `PRIMER.DL.6.FILES.2`:** Inspect file contents with safe read-only tools and confirm expected text. The student uses `ls -l` to inspect metadata, interprets permission records, and verifies that permission changes do not alter file contents.

## Key ideas

Permissions answer a question about an attempted operation: **may this process perform this action on this path?** Linux answers using the process's identity and the path's metadata.

A process acts with:

- a **user identity**;
- a primary group and possibly additional **group memberships**; and
- the capabilities allowed by the matching permission class and other security rules.

A filesystem path has an owning user, an owning group, and permission bits. This lesson does not run `whoami` or `id`; those tools are not required in the activity runtime. Identity is taught as a concept and observed indirectly through owner/group labels in `ls -l`. A Primer student profile is an application identity. It is not the same thing as the operating-system identity the kernel uses for a filesystem permission check.

### Read `ls -l` from left to right

A long listing resembles this:

```console
$ ls -l
-r--r----- 1 learner team 21 Jan  1 12:00 team-template.txt
drwxr-xr-x 2 learner team 33 Jan  1 12:00 reference-shelf
```

Actual owner and group names depend on the isolated runtime, so do not memorize the example names. Read the structure:

```text
- rw- r-- r--
│  │   │   └── other permissions
│  │   └────── group permissions
│  └────────── user (owner) permissions
└───────────── path type
```

The first character is not a permission bit:

- `-` means a regular file;
- `d` means a directory.

The next nine characters form three triplets in fixed order:

1. **user** (`u`) — the path's owner;
2. **group** (`g`) — members of the path's owning group; and
3. **other** (`o`) — identities matching neither of the first two classes.

In a triplet, a letter means the permission is present and `-` means it is absent. The kernel selects the applicable class; it does not search all three triplets for the most generous answer.

## File permissions

For a regular file:

- `r` — read the file's content;
- `w` — change the file's content;
- `x` — request execution of the file as a program or script.

`x` does not prove that a file is trustworthy, safe, or even valid to execute. It is permission, not a quality judgment.

Examples:

| Symbolic record | Meaning |
|---|---|
| `-r--r--r--` | regular file; everyone may read; nobody may write or execute |
| `-rw-r--r--` | owner may read/write; group and other may read |
| `-r--rw----` | owner may read; group may read/write; other has no permissions |

## Directory permissions

The same letters apply differently to a directory because a directory is a mapping from names to entries:

- `r` — list the names in the directory;
- `w` — create, remove, or rename directory entries, subject to other required permissions;
- `x` — traverse/search the directory and access an entry by name.

The directory execute distinction is essential. A directory is not run like a program. Its `x` bit permits passage through that directory in a path.

A directory with read but no execute can produce a surprising result. A command may learn that a name such as `catalog.txt` exists, yet fail to obtain that entry's metadata or content because it cannot traverse from the directory to the named entry. In the lab, `ls -l practice-shelf` shows this distinction: after `a-x`, GNU `ls` can print an unreadable placeholder row but exits with status 1 because it cannot inspect `practice-shelf/catalog.txt`.

Directory permissions interact. Do not reduce them to slogans such as “write means I can always delete.” Operations on directory entries may require combinations of directory write and execute, and parent-directory permissions also matter.

## Limited symbolic `chmod`

`chmod` changes permission bits. The limited symbolic form used here has three parts:

```text
class operation permission
  u       +        w
```

Classes:

- `u` — user/owner class;
- `g` — group class;
- `o` — other class;
- `a` — all three classes.

Operations:

- `+` — add the named bit while preserving unnamed bits;
- `-` — remove the named bit while preserving unnamed bits.

Lesson forms:

```console
$ chmod u+w owner-draft.txt
$ chmod g+w team-draft.txt
$ chmod a-x practice-shelf
$ chmod u+x practice-shelf
```

Read them aloud:

- “add user write to `owner-draft.txt`”;
- “add group write to `team-draft.txt`”;
- “remove execute/traversal from all classes on `practice-shelf`”;
- “add execute/traversal for the user class on `practice-shelf`.”

These commands do not change content, ownership, group ownership, or path type. They alter only the named permission bits.

This lesson deliberately avoids numeric modes as student commands, recursive permission changes, ownership commands, ACLs, special bits, and `chmod 777`. The activity contract uses octal strings such as `0644` only for deterministic fixture setup and `path_mode` verification. Students make narrow symbolic changes because the notation states intent and supports least privilege.

## Vocabulary

- **identity** — the operating-system user and group context under which a process acts.
- **process** — a running instance of a program; filesystem access checks apply to its identity.
- **owner** — the user identity associated with a path's user permission class.
- **owning group** — the group associated with a path's group permission class.
- **user class (`u`)** — the first permission triplet, applying to the path owner.
- **group class (`g`)** — the second permission triplet, applying through the owning group.
- **other class (`o`)** — the third permission triplet, applying when user and group classes do not match.
- **read (`r`)** — content reading for a file; entry-name listing for a directory.
- **write (`w`)** — content modification for a file; directory-entry modification for a directory, with required companion permissions.
- **execute (`x`)** — execution permission for a regular program/script file; traversal/search permission for a directory.
- **mode** — a path's permission bits, often displayed symbolically or represented in octal.
- **symbolic mode** — a `chmod` expression such as `u+w` naming a class, operation, and permission.
- **least privilege** — grant only the authority needed for the intended task.
- **disposable copy** — a practice path created so potentially disruptive changes do not affect source material.
- **fixture** — resettable practice data materialized from the activity definition.

## Guided practice

The activity opens in `workspace/permission-lab` with writable parent directories and safe source fixtures:

```text
permission-lab/
├── owner-template.txt     mode 0444
├── team-template.txt      mode 0440
└── reference-shelf/       mode 0755
    └── catalog.txt        mode 0444
```

All parent directories are explicitly materialized with mode `0755` before files and restrictive child modes. This ensures the fixture tree can be created deterministically: the materializer creates parents first, writes file contents, then forces each declared mode despite the process umask.

Complete five tasks in order.

### 1. Inspect permission records

Run:

```console
$ ls -l
```

For each row:

1. identify `-` or `d` as path type;
2. divide the next nine characters into user/group/other;
3. identify the owner and group columns; and
4. state what each granted letter permits for that path type.

Do not assume the displayed owner name is the student's Primer name. It identifies ownership inside the sandbox filesystem.

### 2. Make disposable copies

Run exactly:

```console
$ cp owner-template.txt owner-draft.txt
$ cp team-template.txt team-draft.txt
```

GNU `cp` preserves these source permission bits on newly created destination files in this environment, so the drafts begin as `0444` and `0440`. The activity verifies those starting modes before later tasks proceed. The source templates remain unchanged.

### 3. Add narrow file permissions

Run:

```console
$ chmod u+w owner-draft.txt
$ chmod g+w team-draft.txt
```

Predict before executing:

- `owner-draft.txt`: `0444` becomes `0644` because only user write is added;
- `team-draft.txt`: `0440` becomes `0460` because only group write is added.

The path contents remain exact. Permission work is metadata work, not content editing.

### 4. Observe directory execute

Run:

```console
$ cp -R reference-shelf practice-shelf
$ chmod a-x practice-shelf
$ ls -l practice-shelf
```

The directory copy starts as `0755`. Removing `x` from all classes leaves `0644`: every class retains read according to its existing bits, but no class has traversal. The final `ls` command is expected to exit with status 1. It can read the directory's list of names but cannot traverse to `catalog.txt` for complete metadata. This expected failure is the evidence.

The check reads `practice-shelf/catalog.txt` through the verifier outside the student's permission-limited shell context to prove its contents still exist and remain exact; the student's command is not expected to read it while traversal is absent.

### 5. Restore only user traversal

Run:

```console
$ chmod u+x practice-shelf
$ ls -l practice-shelf
```

The directory becomes `0744`. The user class now has `rwx`, while group and other retain read without execute. Because the activity process owns the copied directory, user traversal permits the repeated long listing to obtain and display `catalog.txt` metadata successfully.

Explain the contrast:

- on `practice-shelf`, `u+x` lets the owner traverse the directory;
- on a regular script file, `u+x` would let the owner request execution of the file.

## Mastery evidence

The activity uses five ordered tasks and deterministic evidence:

1. Exact `ls -l` command observation plus baseline `path_mode` checks establishes long-list inspection of known permission records.
2. Exact observation of the second `cp` command, path type checks, and copied-mode checks establish both disposable drafts. Because `command_properties` sees only the latest command, filesystem checks prove the first copy while the latest command proves the second requested copy form.
3. Exact observation of the second `chmod`, final mode checks, and content checks prove both narrow file changes. The earlier `u+w` command is established by the unique `0644` outcome; the latest `g+w` command is observed directly.
4. Exact failed `ls -l practice-shelf` observation with status 1, directory type, mode `0644`, and preserved catalog content establish the directory execute experiment. Filesystem outcomes prove the preceding copy and `a-x` steps.
5. Exact successful repeated `ls -l practice-shelf`, output containing `catalog.txt`, mode `0744`, preserved source content, and final cwd establish restored user traversal.

`path_mode` compares only ordinary permission bits and accepts the four-digit mode strings used here. Content checks ensure `chmod` did not alter text. Source checks ensure all permission experiments stayed on copies.

The verifier cannot directly score a student's spoken explanation of identity selection, triplet meaning, or file-versus-directory execute. A parent or tutor must require that explanation before treating the lesson as conceptually complete.

## Parent or tutor review

Ask the student to answer without running `whoami`, `id`, or additional commands:

1. What identity information does the operating system associate with a running process?
2. Why is that not necessarily the same thing as a Primer student profile?
3. In `-rw-r-----`, what does the first character mean?
4. Divide the remaining nine characters into classes and explain each triplet.
5. Does `other` mean “everyone including the owner”? Explain how classes are selected.
6. What does `r` mean on a regular file? On a directory?
7. What does `w` mean on a regular file? On a directory?
8. What does `x` mean on a regular file? On a directory?
9. Why could `ls -l practice-shelf` learn an entry name but fail to obtain its metadata after `a-x`?
10. Read `chmod g+w team-draft.txt` aloud in full.
11. Which bits does the plus operation preserve?
12. Why did the lesson copy templates before changing modes?
13. Why is `chmod 777` not a careful solution?
14. Which checks prove that text content remained unchanged?
15. Which checks prove that source fixtures retained their modes and contents?

Require precise terms: process, identity, owner, owning group, user/group/other, triplet, permission, traversal, symbolic mode, and least privilege.

## Misconceptions

- **“Identity means the name I typed into an application.”** Applications have profiles, but filesystem permission checks use the operating-system identity carried by the process.
- **“I need `whoami` or `id` before I can understand permissions.”** Those commands can report identity, but the concept and permission record can be taught without them, and they are unavailable here.
- **“The first ten characters are all permissions.”** The first character is path type; only the following nine form rwx triplets.
- **“The three triplets are read, write, and execute sections.”** Each triplet belongs to an identity class and can contain all three permission positions.
- **“Other is always checked in addition to owner and group.”** The applicable class is selected; permissions are not accumulated by taking the best bits from every class.
- **“A dash means no user exists.”** It means one permission is absent in one position.
- **“Directory `x` runs the directory.”** Directories are traversed/searched, not executed as programs.
- **“Directory `r` is enough to open everything inside.”** Read can list names; execute is required to traverse to an entry.
- **“File `x` means safe to run.”** It grants execution permission but makes no trust or correctness claim.
- **“`chmod` edits the file.”** It changes mode bits, not content.
- **“`+` replaces the whole mode.”** It adds named bits and preserves unnamed bits.
- **“Broad permissions are easier and therefore better.”** Broad grants violate least privilege and can expose modification or execution authority unnecessarily.
- **“If a deliberate permission test returns nonzero, the lesson failed.”** The status-1 listing is expected evidence that traversal was removed.

## Tutor strategy and constraints

Use this sequence before every command:

1. Ask what path type is involved: regular file or directory.
2. Ask which identity class is relevant: user, group, other, or all.
3. Ask what operation is intended: inspect, copy, add a bit, or remove a bit.
4. Ask the student to predict the resulting symbolic and octal mode.
5. Reveal the exact current command only at hint level 3.
6. After execution, require interpretation of the result rather than accepting a passed check alone.

Keep command coaching within `ls -l`, `cp`, `cp -R`, `chmod u+w`, `chmod g+w`, `chmod a-x`, and `chmod u+x`. Do not suggest `whoami`, `id`, `groups`, `stat`, `sudo`, `su`, `chown`, `chgrp`, `getfacl`, recursive `chmod`, numeric student commands, `umask`, special bits, or ACLs. Do not broaden a mode merely to make an error disappear.

If the directory listing fails after `a-x`, ask: “Which directory capability is missing, and what metadata was `ls -l` trying to obtain?” Treat status 1 as the expected observation. If source templates change, or if extra permissions are granted, reset instead of repairing with increasingly broad commands.

## Safety and craftsmanship

- Work only in `workspace/permission-lab`.
- Inspect before changing.
- Preserve `owner-template.txt`, `team-template.txt`, `reference-shelf`, and `reference-shelf/catalog.txt`.
- Change modes only on `owner-draft.txt`, `team-draft.txt`, and `practice-shelf`.
- Use symbolic changes that name exactly one intended class and permission, except the deliberate bounded `a-x` experiment.
- Never use `chmod 777` or recursive `chmod` in this lesson.
- Never alter workspace ancestors; removing directory execute from the cwd could make the practice environment unusable.
- Predict the resulting mode before every `chmod`.
- Distinguish expected permission denial from a missing path or spelling error.
- Reset if the fixture boundary or requested mode is violated.

## Extension

Use verbal or paper answers only; do not modify the assessed workspace.

For each record, identify path type and explain user, group, and other permissions:

1. `-rw-r-----`
2. `-rwxr-x---`
3. `drwxr-xr-x`
4. `drw-r-----`
5. `dr-x------`
6. `-r--rw----`

Then answer:

1. Which symbolic change adds owner write to `notes.txt` without changing other bits?
2. Which symbolic change removes group write from `shared.txt`?
3. Why might `drw-r--r--` reveal names but block access by those names?
4. Why does adding `x` to a directory not make it a program?
5. Why should a source template be copied before a permission experiment?
6. Give one example where a broad permission grant would exceed the task's need.

For every answer, require a least-privilege justification: identify the exact class, bit, path type, intended operation, and permissions that must remain unchanged.
