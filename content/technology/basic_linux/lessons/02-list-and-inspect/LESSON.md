# Lesson 02: List and inspect

## Objectives

By the end of this lesson, the student can:

- use `ls` to list ordinary names in the current directory;
- use `ls -a` to include hidden names;
- use `ls -l` to inspect long-list metadata;
- distinguish a regular file from a directory by reading the first character of a long listing;
- use `cat` to print the contents of a short text file; and
- explain when `file` would be useful, even though it is not installed in this lesson's runtime.

## Key ideas

A directory contains **names** that point to files and subdirectories. Running `ls` asks the shell to execute the listing program. With no path argument, `ls` lists the current directory. It observes the workspace without changing it.

An option changes how a command behaves:

```console
$ ls
field-notes  README.txt
$ ls -a
.  ..  .manifest.txt  field-notes  README.txt
```

Linux normally treats a name beginning with `.` as hidden from an ordinary `ls` listing. “Hidden” is a naming convention, not encryption or a separate kind of file. `ls -a` means “all” and includes dot-prefixed names. It also commonly displays `.` (the current directory) and `..` (the parent directory).

`ls -l` produces a **long listing**. A typical line resembles this:

```text
-rw-r--r-- 1 student student 58 Jan 10 09:00 observations.txt
drwxr-xr-x 2 student student  6 Jan 10 09:00 archive
```

Read the first character first:

- `-` means a regular file;
- `d` means a directory.

The remaining columns are metadata: permission symbols, link count, owner, group, size, modification time, and name. Exact owner names, sizes, spacing, and timestamps can differ between systems, so focus first on the type character and final name. A directory's displayed size is metadata about the directory entry, not the total size of everything inside it.

`cat PATH` prints a file's contents to the terminal. It is appropriate for the short text fixtures in this lesson. It does not tell the shell to enter a directory, and using it on a directory produces an error.

On many Linux systems, `file PATH` inspects data and reports a likely file format, such as text or an image. The `file` program is unavailable in the `coreutils-basic` runtime, so do not try to run it here. Discuss its purpose only; use `ls -l` for file-versus-directory metadata and `cat` for the supplied short text files.

## Vocabulary

- **listing** — names and, optionally, details reported for a directory.
- **option** — a command argument, often beginning with `-`, that changes command behavior.
- **hidden name** — a Linux name whose first character is `.`, omitted by ordinary `ls` output.
- **metadata** — information about a filesystem object, such as type, permissions, size, owner, or modification time.
- **regular file** — a filesystem object that stores data; shown with `-` at the start of an `ls -l` line.
- **directory** — a filesystem object that organizes names; shown with `d` at the start of an `ls -l` line.
- **contents** — the data stored inside a file.
- **path** — a name or sequence of names that identifies a filesystem location.

## Guided practice

The activity opens in `workspace`, a read-only inspection scene. Run each command separately and pause to interpret its output.

1. Run `ls`. Locate `README.txt` and `field-notes`. Predict whether `.manifest.txt` will appear.
2. Run `ls -a`. Find the newly visible dot-prefixed name. Notice that `.` and `..` may also appear.
3. Run `ls -l field-notes`. Ignore exact timestamps and owner names. Match each final name to the first character on its line. Confirm that `archive` begins with `d`, while `observations.txt` and `supplies.txt` begin with `-`.
4. Run `cat field-notes/observations.txt`. Read the short report printed to standard output.
5. Without running it, explain what question `file field-notes/observations.txt` could help answer on a system where `file` is installed.

## Mastery evidence

The terminal activity collects deterministic evidence from four ordered tasks. The student must:

1. run plain `ls` and produce a listing containing the expected ordinary names;
2. run `ls -a` and reveal `.manifest.txt`;
3. run `ls -l field-notes` and produce long-list lines showing a directory entry and regular-file entries; and
4. run `cat field-notes/observations.txt` and produce the fixture's exact text.

Mastery means choosing the requested command and option, not merely knowing that fixture paths exist. The checks use fixed fixtures and the most recent command/output. A reset restores the same workspace.

Because the current contract cannot score an oral explanation, a parent or tutor should additionally ask: “How can you tell `archive` is a directory, and why did plain `ls` omit `.manifest.txt`?” A correct response refers to the leading `d` in `ls -l` and the initial dot in the hidden name.

## Misconceptions

- **“`ls` opens a directory.”** It lists names; it does not change the current directory.
- **“Hidden means secret or protected.”** A leading dot only changes ordinary listing behavior. Permissions are separate metadata.
- **“`ls -l` shows file contents.”** It shows metadata. Use `cat` for the contents of a short text file.
- **“Every long-list line beginning with `-` is an option.”** In command input, `-l` is an option. In long-list output, a leading `-` is the type marker for a regular file.
- **“A larger directory size means all files inside total that size.”** The displayed directory size is not a recursive total.
- **“`cat` can identify any file safely.”** `cat` simply writes bytes to the terminal and is best here for known short text. It is not a general format detector.
- **“I should run `file` because the course outline names it.”** This lesson's runtime does not provide `file`; its purpose is discussion-only here.

## Extension

Choose another supplied short text file and inspect it in two stages: first use `ls -l` with its path to identify its type and metadata, then use `cat` to read it. Before each command, state whether you expect metadata or contents.

For a verbal challenge, compare these questions and choose the tool that would answer each on a full Linux system:

- “What names are in this directory?” — `ls`
- “Which names begin with a dot?” — `ls -a`
- “Is this entry a regular file or directory?” — `ls -l`
- “What does this short known-text file say?” — `cat`
- “What kind of data might this unfamiliar file contain?” — `file` (discussion only in this runtime)
