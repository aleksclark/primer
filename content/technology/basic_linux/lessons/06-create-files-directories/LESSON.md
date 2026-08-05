# Lesson 06: Create files and directories

## Objectives

By the end of this lesson, the student can:

- use `mkdir NAME` to create one directory in the current working directory;
- use `mkdir -p PATH` to create a nested directory path whose parents may not yet exist;
- use `touch PATH...` to create empty files at existing parent paths;
- choose clear, meaningful names that are easy to type and recognize;
- predict where a relative path will be created; and
- verify a completed structure with `ls -R` rather than assuming a successful result.

## Key ideas

A filesystem-changing command should follow a disciplined cycle: **plan, create, verify**. First identify the current working directory and resolve the path. Next run the smallest command that creates the required object. Finally inspect the result. A command returning silently often means success, but silence is not evidence that the complete structure matches the plan.

`mkdir` means “make directory.” With one simple relative name, it creates a child of the current working directory:

```console
$ mkdir field-notes
```

If the shell is in `/workspace/workspace/build-zone`, the new directory is `/workspace/workspace/build-zone/field-notes`. Plain `mkdir` does not create missing parent directories. For example, `mkdir field-notes/research/sources` works only if `field-notes/research` already exists.

The `-p` option tells `mkdir` to create missing **parent** directories along a path:

```console
$ mkdir -p field-notes/research/sources
```

Here `field-notes` already exists, while `research` and `sources` do not. `mkdir -p` preserves the existing directory and creates the missing parts. It is useful when the intended nested path is known. It should not replace planning: a mistyped component still creates the wrong tree.

`touch` creates an empty regular file when the named path does not exist:

```console
$ touch field-notes/README.txt field-notes/research/sources/web-links.txt
```

Every parent directory must already exist. `touch` does not create missing directories. If a file already exists, `touch` normally updates its timestamps rather than clearing its contents; it is not a text editor. In this lesson both target files are new, so each becomes an empty file.

Names are part of organization. Prefer names that state purpose, such as `research`, `sources`, and `web-links.txt`. Use one consistent style. Hyphens make multiword names clear without requiring quoting, and a useful extension such as `.txt` gives readers a clue about intended content. Avoid vague names such as `stuff`, accidental capitalization changes, and hard-to-distinguish variants. Linux names are case-sensitive: `README.txt` and `readme.txt` are different names.

`ls -R PATH` lists a directory **recursively**: it shows the named directory and then its descendants. In this small, supplied tree it is an appropriate final verification tool. Read the section headers and entries to confirm both object names and nesting. Recursive listing is for this bounded practice tree; it can produce too much output in a large real directory.

## Vocabulary

- **create** — add a new filesystem object at a path.
- **directory tree** — directories and their nested children considered as an organized hierarchy.
- **parent directory** — the directory that directly contains an object.
- **nested path** — a path with multiple directory components below a starting point.
- **`mkdir`** — the command that creates a directory.
- **`-p`** — the `mkdir` option that creates missing parent components and accepts already-existing directories.
- **`touch`** — a command that creates a missing empty file or updates timestamps on an existing file.
- **empty file** — a regular file containing zero bytes of data.
- **clear name** — a consistent, meaningful name that communicates an object's purpose.
- **recursive listing** — a listing that visits a directory and its descendant directories.
- **verification** — observing the resulting state and comparing it with the intended structure.

## Guided practice

The activity opens in the writable fixture directory `workspace/build-zone`. The target structure is:

```text
field-notes/
├── README.txt
└── research/
    └── sources/
        └── web-links.txt
```

Complete one change at a time.

1. Create `field-notes` with plain `mkdir`. Before running it, state that the relative name will become a child of `build-zone`.
2. Create `field-notes/research/sources` with one `mkdir -p` command. Resolve the path from left to right and identify the two missing directory components.
3. Use one `touch` command to create `field-notes/README.txt` and `field-notes/research/sources/web-links.txt`. Notice that their parent directories now exist.
4. Run `ls -R field-notes`. Read each section of the output and compare it with the target tree. Confirm that `README.txt` is directly inside `field-notes`, while `web-links.txt` is inside `research/sources`.

Do not create alternate spellings, extra directories, or extra files. If a name is mistyped, reset the activity rather than using removal or renaming commands that have not yet been taught.

## Mastery evidence

The terminal activity uses four ordered, deterministic tasks. It checks that the student:

1. runs plain `mkdir field-notes` and creates a directory at the correct relative path;
2. runs `mkdir -p field-notes/research/sources` and creates both required nested directories;
3. runs one exact `touch` command for the two requested clear file names and creates regular files at both paths; and
4. runs `ls -R field-notes`, produces output containing the expected names, and leaves the complete tree in the required types and locations.

Command checks establish use of `mkdir`, `mkdir -p`, `touch`, and final verification. Filesystem checks establish the resulting structure independently of terminal formatting. The fixture is fixed, and reset returns to an empty, writable `build-zone` containing only its instruction card.

The current contract cannot score the student's spoken planning. A parent or tutor should additionally ask: “Why was `-p` useful for the second command, why would `touch` have failed before the directories existed, and how did the final listing prove the nesting?” A mastery response identifies missing parents, the requirement for existing parent directories, and the recursive listing's section headers and entries.

## Parent or tutor review

Ask the student to point to each component of `field-notes/research/sources/web-links.txt` and explain whether it names a directory or file. Then ask:

1. What would plain `mkdir field-notes/research/sources` have done before `research` existed?
2. What does `mkdir -p` do if an earlier directory in the requested path already exists?
3. Does `touch` put words into a newly created file?
4. Why are `web-links.txt` and `research` clearer names than `stuff` and `new`?
5. Why is verification a separate step from creation?

Require precise vocabulary: parent, path component, directory, regular file, empty file, and verification.

## Misconceptions

- **“`mkdir` creates a file.”** It creates a directory. `touch` creates the empty regular files in this lesson.
- **“Plain `mkdir` creates every missing part of a path.”** It requires the parent path to exist. `mkdir -p` creates missing parents.
- **“`-p` means permanent.”** Here it refers to parent-directory behavior; it does not protect the result from later changes.
- **“`mkdir -p` fixes spelling mistakes.”** It faithfully creates the components typed, including incorrect ones.
- **“`touch` creates missing parent directories.”** The parent of every target file must already exist.
- **“`touch` writes the filename into the file.”** A newly created file is empty; its name is directory metadata, not its contents.
- **“`touch` empties an existing file.”** It normally updates timestamps and preserves existing contents.
- **“Names differing only by capitalization are the same.”** Linux treats `README.txt` and `readme.txt` as distinct names.
- **“No output proves the whole job is correct.”** Verify the state explicitly after creating it.
- **“A recursive listing is always best.”** It is suitable for this small bounded tree but can overwhelm the terminal in a large tree.

## Safety and craftsmanship

- Work only inside the supplied `workspace/build-zone` fixture.
- Use relative paths exactly as requested; do not create anything at `/` or elsewhere in the sandbox.
- All fixture directories are mode `0755`, so they remain writable and traversable while the runner materializes the fixture and while the student creates descendants.
- Create only the specified tree. Precision includes not leaving accidental names behind.
- Do not use `rm`, `rmdir`, `mv`, or `cp`; those commands belong to later lessons.
- If the structure becomes incorrect, use the activity reset and rebuild carefully.

## Extension

After mastering the assessed activity, reset it and verbally design a second tree for `experiment-notes` with nested `observations/daily` directories and empty files named `README.txt` and `day-01.txt`. Before running anything, write or say which command could create the first directory, which single command could create the missing nested parents, and when the file parents would be ready for `touch`.

As an unscored naming review, compare `day-01.txt`, `Day 1`, `newfile`, and `observations-for-first-day.txt`. Discuss consistency, meaning, ease of typing, spaces that require careful quoting, and whether a shorter clear name can remain precise. Do not create the extension tree unless a parent explicitly asks for additional sandbox practice.
