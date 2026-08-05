# Linux and CLI Fundamentals

This course is a mastery sequence sized as approximately four weeks of one-hour daily work. The day labels indicate content volume and a sensible order, not calendar pacing. A student advances only after demonstrating the lesson objectives and should repeat, pause, or receive additional practice as needed.

## Week 1: Orientation and navigation

1. **Meet the command line** — Distinguish the terminal, shell, command, argument, input, and output; run `pwd`, `whoami`, and `echo` while learning to read the prompt.
2. **List and inspect** — Use `ls`, `ls -l`, `ls -a`, `file`, and `cat` to identify ordinary files, hidden names, directories, and basic metadata.
3. **Paths and movement** — Navigate with `cd`, `.`, `..`, relative paths, and absolute paths; explain the current working directory after each move.
4. **The Linux filesystem** — Explore a safe model of `/`, `/home`, `/etc`, `/var`, `/tmp`, `/usr`, and `/bin`; connect each directory to the operating system's organization.
5. **Navigation mastery challenge** — Locate specified files in an unfamiliar tree, report paths precisely, and demonstrate navigation without trial-and-error wandering.

## Week 2: Files, directories, and safe changes

6. **Create files and directories** — Build a directory tree with `mkdir`, `mkdir -p`, and `touch`; use clear names and verify the result.
7. **Copy, move, and rename** — Use `cp` and `mv` to duplicate, relocate, and rename files while predicting each command's effect.
8. **Remove safely** — Use `rm`, `rmdir`, and limited recursive removal in a sandbox; distinguish recoverable practice from destructive real-system actions.
9. **Read and compare text** — Inspect text with `cat`, `head`, `tail`, `wc`, and `diff`; choose the smallest useful command for a question.
10. **Permissions and identity** — Interpret user/group/other and read/write/execute bits, inspect modes with `ls -l`, and make limited changes with `chmod`.

## Week 3: Finding, combining, and understanding commands

11. **Find files by name** — Use `find` with safe starting paths and name/type predicates; explain why search scope matters.
12. **Search inside files** — Use `grep` for literals and simple patterns, include line numbers, and search several files without opening each one.
13. **Pipes and redirection** — Explain standard input, output, and error; combine commands with `|`, `>`, `>>`, and `2>` without losing important data.
14. **Process text** — Use `sort`, `uniq`, `cut`, `tr`, and `wc` in short pipelines to answer questions about structured text.
15. **Commands, processes, and exit status** — Distinguish shell builtins from programs, inspect environment variables, use `ps`, and reason about success, failure, and `$?`.

## Week 4: Limited Bash scripting and capstone

16. **Your first Bash script** — Create an executable script with a shebang, comments for human readers, simple commands, and a predictable output file.
17. **Variables and arguments** — Quote variables correctly, read positional arguments, and use `printf` to produce stable output.
18. **Conditions and loops** — Use `test`/`[ ]`, `if`, and a bounded `for` loop to process known files safely.
19. **Capstone build: system field report** — Build a `field-report` project that inventories a supplied Linux-like workspace and produces organized, reproducible text reports.
20. **Capstone verification and explanation** — Test the project from a clean fixture, compare expected and actual output, correct defects, and explain the filesystem, pipeline, permission, and scripting choices used.

## Capstone outcome

The completed `field-report` project contains an executable `report.sh`, a `README.md`, and generated reports. Given a supplied workspace, the script creates a report directory, inventories files by type, searches logs for notable entries, summarizes counts, and preserves errors separately. The student must be able to run it from a clean starting state and explain every command rather than merely produce matching files.

## Content layout

Each numbered lesson has:

- a Markdown teaching brief for parent review and future richer presentation;
- a JSON activity document directly shaped for Primer's student-client activity contract;
- deterministic tasks, checks, graduated hints, and tutor constraints.

`activity.schema.json` is the authoring schema. `course.json` defines course order for the loader. See `SCHEMA_ASSESSMENT.md` for compatibility decisions and `PLATFORM_GAPS.md` for capabilities still needed for the complete instructional experience.

## Validate and load

The loader validates every JSON activity before making a request. It uses only Python's standard library for HTTP and the `jsonschema` package for local validation.

```bash
python3 content/technology/basic_linux/load.py --validate-only

export PRIMER_API=https://primer.example/api/v1
export PRIMER_PARENT_EMAIL=parent@example.com
export PRIMER_PARENT_PASSWORD=secret
python3 content/technology/basic_linux/load.py --student-id <student-uuid>
```

A parent token may be supplied as `PRIMER_PARENT_TOKEN` or `--token` instead of credentials. Use `--dry-run` to list actions without contacting Primer and `--no-assign` to publish globally without targeting a student. Loading is repeatable: content-identical publication resolves to the immutable existing revision, and assign-next preserves an existing open assignment for the slug.
