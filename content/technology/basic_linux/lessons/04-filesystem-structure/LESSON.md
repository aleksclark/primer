# Lesson 04: The Linux filesystem

## Objectives

By the end of this lesson, the student can:

- explain that `/` is the filesystem root, the top of one Linux directory tree;
- connect `/home`, `/etc`, `/var`, `/tmp`, `/usr`, and `/bin` with their common organizational roles;
- distinguish the real filesystem root from the lesson's safe `simulated-root` fixture;
- use relative paths with `ls` and `cat` to inspect the model without changing it; and
- treat directory roles as conventions rather than guarantees about every file or Linux distribution.

## Key ideas

Linux organizes files in one directory tree. The top is the **filesystem root**, written `/`. The slash is both the name used for that top location and the separator between path components. Every absolute path begins there: `/etc`, for example, means the `etc` directory immediately below `/`.

This lesson does **not** inspect the sandbox's real `/`. Its fixture contains a directory named `simulated-root` that safely models selected root-level directories:

```text
simulated-root/             model of /
├── bin/                    model of /bin
├── etc/                    model of /etc
├── home/                   model of /home
├── tmp/                    model of /tmp
├── usr/                    model of /usr
└── var/                    model of /var
```

The names have common roles:

- **`/home`** commonly holds users' personal directories and files. A user named Ada might have a home directory such as `/home/ada`.
- **`/etc`** commonly holds system-wide configuration: settings used by the operating system and installed services. It is not a general folder for a user's documents.
- **`/var`** commonly holds data that changes while the system runs, such as logs, queues, caches, and service state. “Variable” describes changing data, not shell variables.
- **`/tmp`** is for temporary working files. Programs or system cleanup may remove them, so it is not dependable long-term storage. Temporary also does not automatically mean private.
- **`/usr`** commonly contains installed programs, libraries, and shared read-only data. The name is historical; it should not be interpreted as each user's personal home.
- **`/bin`** commonly provides command programs needed for basic operation, such as `ls` and `cat`. On many modern systems `/bin` is linked into `/usr`, but its basic-program role remains useful when reading paths.

These are organizational conventions, not claims that every distribution has an identical layout. A role tells what normally belongs somewhere; it does not prove the purpose, safety, or permissions of every item found there.

The fixture uses `ROLE.txt` cards as labels. Real Linux root directories do not normally contain these lesson cards. Inspect only paths beginning with `simulated-root/`; a leading `/` would instead address the sandbox's actual root.

## Vocabulary

- **filesystem** — the organized tree of directories and files available to the operating system.
- **filesystem root (`/`)** — the top directory of the filesystem tree and the starting point of every absolute path.
- **root-level directory** — a directory immediately below `/`, such as `/etc` or `/home`.
- **directory role** — the conventional purpose of a directory in the system's organization.
- **configuration** — settings that control how a system or program behaves.
- **variable data** — data expected to change during operation, such as logs or queues.
- **temporary file** — short-lived working data that must not be assumed to persist.
- **program** — executable instructions that a command can run.
- **shared data** — resources, such as libraries or documentation, intended for use by multiple programs or users.
- **simulated root** — this lesson's ordinary fixture directory that models `/` without exposing or changing the real root.

## Guided practice

The activity opens in `workspace`. Stay there and use only relative paths beginning with `simulated-root/`. All commands are read-only.

1. Run `ls simulated-root`. Read the six names as models of directories directly beneath `/`. State aloud why `simulated-root` is only a model and is not itself `/`.
2. Run `cat simulated-root/home/ROLE.txt`. Connect personal user space with `/home`, while noticing that the practice card is inside the safe model.
3. In one `cat` command, read `simulated-root/etc/ROLE.txt`, `simulated-root/var/ROLE.txt`, and `simulated-root/tmp/ROLE.txt`, in that order. Contrast durable configuration, changing operational data, and disposable working data.
4. In one `cat` command, read `simulated-root/usr/ROLE.txt` and `simulated-root/bin/ROLE.txt`, in that order. Contrast the broad collection of installed programs/shared resources with the traditional location for basic commands.

After each command, point to the path component that names the modeled root-level directory. Do not use `cd /`, `ls /`, or any command that creates, edits, moves, or removes data.

## Mastery evidence

The terminal activity uses four ordered tasks with fixed fixtures and deterministic checks. The student must:

1. use `ls simulated-root` and produce a listing containing all six modeled root-level directory names;
2. use `cat simulated-root/home/ROLE.txt` and produce the exact `/home` role card;
3. read the `/etc`, `/var`, and `/tmp` role cards in the requested order with one exact `cat` command; and
4. read the `/usr` and `/bin` role cards in the requested order with one exact `cat` command.

The final task also verifies that the fixed role cards remain unchanged. A reset recreates the same fixture. No task reads or changes the host or sandbox root; all assessed paths are workspace-relative fixture paths.

The current contract can score commands and output but cannot score the conceptual explanation. A parent or tutor should additionally ask: “If a program needs a system-wide setting, a changing log, a short-lived scratch file, and one user's personal document, which four directories would normally fit, and why?” A correct response chooses `/etc`, `/var`, `/tmp`, and `/home` and distinguishes their roles. The student should also explain that this activity used `simulated-root`, not the real `/`.

## Misconceptions

- **“`simulated-root` is the real `/`.”** It is an ordinary directory inside the assigned workspace. It only models selected children of `/`.
- **“Root means a user account here.”** The filesystem root `/` is a directory. The administrative user named `root` is a separate concept.
- **“`/home` contains every person's files everywhere.”** It commonly contains users' personal directories, but layouts and service accounts vary.
- **“`/etc` means miscellaneous leftovers.”** On Linux it conventionally holds system-wide configuration.
- **“`/var` is where shell variables are stored.”** It normally holds data that varies while the system runs.
- **“Files in `/tmp` are permanent or automatically private.”** Temporary data may be cleaned up and still requires appropriate permissions.
- **“`/usr` is one user's private folder.”** It generally contains installed software and shared resources; personal directories usually belong under `/home`.
- **“Every command must physically live in `/bin`.”** `/bin` has the traditional basic-command role, but modern systems may link it into `/usr` or organize programs differently.
- **“Knowing a directory's role makes any command there safe.”** A role describes organization, not permission to inspect or modify a real system.

## Extension

Without leaving the fixture, make a verbal placement plan for four imaginary items: Ada's essay, a service configuration file, today's service log, and a program's disposable scratch file. Assign them to the modeled `/home`, `/etc`, `/var`, and `/tmp` directories and justify each choice. Do not create the files.

For a second challenge, compare `/bin` and `/usr`: explain why both can be associated with programs, then state the useful introductory distinction between basic command programs and the broader collection of installed software, libraries, and shared data. Finally, explain why a modern system might link `/bin` into `/usr` without making the path `/bin` meaningless.
