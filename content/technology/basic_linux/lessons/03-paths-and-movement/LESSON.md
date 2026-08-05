# Lesson 03: Paths and movement

## Objectives

By the end of this lesson, the student can:

- explain that the current working directory is the directory a shell uses as the starting point for relative paths;
- use `pwd` to report the current working directory;
- use `cd` to move through a supplied directory tree without changing its contents;
- interpret `.` as the current directory and `..` as its parent;
- distinguish a relative path from an absolute path; and
- predict the current working directory after each move.

## Key ideas

A shell is always working in one directory. That location is the **current working directory**, often shortened to **cwd**. Commands that receive a relative path begin resolving it from the cwd. Run `pwd` (“print working directory”) whenever you need to confirm that starting point.

`cd PATH` means “change directory.” It changes the shell's cwd; it does not move, rename, or edit the directory itself. For example, if the cwd is `/workspace/workspace`, then:

```console
$ cd ./expedition/maps
$ pwd
/workspace/workspace/expedition/maps
```

A **relative path** describes a location from the current working directory. It does not begin with `/`. In `./expedition/maps`, the first component `.` means “this current directory.” Writing `expedition/maps` would identify the same destination from this starting point.

The special component `..` means “the parent directory,” one level above the cwd. From `/workspace/workspace/expedition/maps`, `cd ..` changes the cwd to `/workspace/workspace/expedition`. It does not mean “the directory used earlier”; its meaning depends on the cwd at the moment the shell resolves it.

An **absolute path** starts with `/` and describes a location from the filesystem root rather than from the cwd. In this sandbox, the practice fixture is mounted below `/workspace`, so `/workspace/workspace/expedition/maps` identifies the maps directory no matter whether the cwd is `workspace`, `expedition`, or `logs`. Absolute paths are precise but can be longer and may differ on another system. Relative paths are often clearer for nearby destinations but change meaning when the cwd changes.

A path can contain several **components**, separated by `/`. Resolve a relative path from left to right. For example, from the lesson's starting directory:

```text
./expedition/maps
│  │          └── enter maps
│  └───────────── enter expedition
└──────────────── begin at the current directory
```

Before pressing Enter on a `cd` command, predict the destination. Afterward, use the prompt or `pwd` to check the prediction. This habit prevents trial-and-error wandering.

## Vocabulary

- **current working directory (cwd)** — the directory the shell currently uses as the starting point for relative paths.
- **`pwd`** — a command that prints the current working directory's absolute path.
- **`cd`** — the shell command that changes the current working directory.
- **path** — a sequence of names and special components that identifies a filesystem location.
- **path component** — one part of a path between `/` separators.
- **relative path** — a path resolved from the current working directory; it does not begin with `/`.
- **absolute path** — a path that begins with `/` and is resolved from the filesystem root.
- **`.`** — the special path component meaning the current directory.
- **`..`** — the special path component meaning the parent of the current directory.
- **parent directory** — the directory one level above another directory.

## Guided practice

The activity opens in the safe fixture directory `/workspace/workspace`. The fixture contains an `expedition` directory with neighboring `maps` and `logs` directories. All practice changes only the shell's cwd.

1. Run `pwd`. Read the absolute path and identify the final `workspace` component as the starting cwd.
2. Predict the result of `cd ./expedition/maps`, then run it. Here `.` names the starting cwd and the remaining components lead downward through the tree.
3. From `maps`, run `cd ..`. State why the cwd is now `expedition`, then verify the prediction from the displayed location or with `pwd` during unscored exploration.
4. From `expedition`, use the relative path `logs` with `cd`. Notice that a nearby child needs only its name; `/workspace/workspace/expedition/logs` would name the same destination absolutely.
5. From `logs`, return to `maps` with the absolute path `/workspace/workspace/expedition/maps`. Explain why that path reaches the same target regardless of the cwd.

Keep a short verbal trace: “I started in workspace; I followed a relative path into maps; `..` took me to expedition; the relative child name took me into logs; the absolute path returned me to maps.”

## Mastery evidence

The terminal activity uses five ordered tasks with fixed fixtures and deterministic checks. The student must:

1. run `pwd` and report the sandbox's starting absolute path;
2. use `cd ./expedition/maps` and reach the nested maps directory;
3. use `cd ..` and reach its parent, expedition;
4. use the relative child path `logs` and reach the neighboring logs directory; and
5. use `/workspace/workspace/expedition/maps` as an absolute `cd` target and finish in maps.

Each movement task checks both the requested form of the `cd` command and the resulting cwd. The fixed tree is restored on reset, and no task requires creating, editing, moving, or deleting a file.

The current activity contract can verify commands and cwd but cannot score a spoken explanation. A parent or tutor should additionally ask: “Why can `logs` and `/workspace/workspace/expedition/logs` identify the same directory in one moment, and why might `logs` identify somewhere else after the cwd changes?” A correct answer contrasts resolution from the cwd with resolution from `/`.

## Misconceptions

- **“`cd` moves the directory.”** `cd` changes the shell's current working directory; the filesystem tree stays where it was.
- **“The prompt always tells me enough.”** Prompt formats vary. `pwd` gives an explicit absolute path.
- **“`.` means the filesystem root.”** `.` means the current directory. `/` is the filesystem root.
- **“`..` means go back to wherever I was before.”** It means the current directory's parent, not command history.
- **“Every path beginning with a dot is hidden.”** A name such as `.notes` is conventionally hidden, but the standalone components `.` and `..` have navigation meanings.
- **“A relative path always reaches the same place.”** Its meaning depends on the cwd from which it is resolved.
- **“An absolute path works on every computer.”** It is independent of the current cwd, but the same directory tree may not exist on another system.
- **“A leading `/` means the lesson workspace.”** It means the filesystem root. This sandbox's fixture happens to be mounted under `/workspace`.

## Extension

Without changing the fixture, find two additional ways to describe `maps` while standing in `expedition`: the simple relative child name and the relative path beginning with `./`. Predict whether both resolve to the same cwd, test them one at a time, and confirm with `pwd`.

For a verbal challenge, begin at `/workspace/workspace/expedition/maps` and resolve `../logs` component by component. Explain why `..` first reaches `expedition` and `logs` then enters its child. Compare that concise relative path with the absolute path `/workspace/workspace/expedition/logs`.
