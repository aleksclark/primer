# Lesson 19: Capstone build — system field report

## Purpose

This capstone asks the student to combine the course's major habits in one useful, reviewable build. The student receives a small Linux-like tree, surveys it, and creates a `field-report` project containing:

- `README.md` — brief operating documentation;
- `report.sh` — an executable, readable Bash script;
- `reports/config-files.txt` — an ordered configuration-file inventory;
- `reports/errors.txt` — notable entries from the named current log;
- `reports/categories.txt` — distinct inventory categories;
- `reports/summary.txt` — stable counts for the fixture.

The goal is not merely matching output. The student must resolve relative paths, distinguish `find` from `grep`, trace pipelines, choose redirection deliberately, limit Bash to understandable constructs, preserve source data, and explain why another unchanged run is reproducible.

## Objectives

By the end of this lesson, the student can:

- navigate into and back out of a nested fixture using relative paths;
- distinguish the safe fixture from similarly named directories on a real Linux system;
- use a bounded `find` search to inventory regular files by scope, type, and name;
- use `grep` on one explicitly named log with an anchored, case-sensitive pattern;
- trace `cut | sort | uniq` and `find`/`grep | wc -l` pipelines from left to right;
- create a clear project directory and short operating README;
- build and inspect a limited Bash script using a shebang, comments, simple variables, quoted expansions, pipelines, and redirection;
- use `>` to replace generated reports and use `>>` only after a fresh replacement in the same run;
- add only user execute permission, run the script from its documented directory, and verify exact generated results;
- preserve every supplied source file and defend the design orally.

## Standards

- **Primary — `PRIMER.DL.6.FILES.1`:** Create the `field-report` directory, maintain its script and documentation at explicit paths, and organize generated results under `reports/`.
- **Primary — `PRIMER.DL.6.FILES.2`:** Inspect fixture text, script text, and generated reports with safe read-only tools; verify exact output and source preservation.
- **Reinforcement — `PRIMER.DL.6.NAV.2`:** Follow relative paths into the fixture and project, distinguish files from directories, and explain path resolution from the current directory.

Scripting-specific standards are still pending, so the activity records `standardsStatus: pending-scripting-standards` while using the closest seeded standards.

## Supplied safe fixture

The starting workspace is `workspace/capstone-lab`. It contains only this synthetic, read-only tree:

```text
fixture/system/
├── etc/
│   ├── app/
│   │   ├── main.conf
│   │   └── worker.conf
│   └── network.conf
├── home/
│   └── operator/
│       └── notes.txt
├── usr/
│   └── share/
│       └── docs/
│           └── runbook.txt
└── var/
    ├── lib/
    │   └── inventory/
    │       └── assets.csv
    └── log/
        ├── archive.log
        └── operations.log
```

These names resemble conventional Linux directories so the student can practice interpreting a system-like organization. They are not `/etc`, `/var`, `/usr`, or `/home`; they are ordinary lesson directories under the workspace. No command should begin at `/`, name a real absolute system path, or leave `workspace/capstone-lab`.

The fixture includes deliberate distinctions:

- three lowercase `.conf` regular files;
- eight regular files total;
- two uppercase `ERROR` lines in `operations.log`;
- one lowercase `error` decoy in that same log;
- one archived uppercase error outside the requested current-log scope;
- repeated categories in `assets.csv`, so `sort` must group values before `uniq` removes adjacent duplicates.

## Core design

### 1. Location determines relative-path meaning

The script is documented to run from `field-report`:

```text
workspace/capstone-lab/field-report/
```

From there:

```bash
source_dir='../fixture/system'
report_dir='reports'
```

`..` goes from `field-report` to `capstone-lab`; `fixture/system` then descends to the source tree. `reports` stays inside the project. These paths are correct because the run location is explicit. Running the script from another directory would resolve them differently.

Every expansion is quoted:

```bash
"$source_dir"
"$report_dir"
```

Quotes preserve each complete path as one shell word and make the intended data boundary visible.

### 2. `find` searches paths; `grep` searches text

The configuration inventory is a bounded filesystem search:

```bash
find "$source_dir" -type f -name '*.conf' | sort
```

Read it left to right:

1. begin only at the supplied source directory;
2. descend beneath that point;
3. keep regular files;
4. keep basenames matching lowercase `*.conf`;
5. sort the resulting paths for deterministic order.

The wildcard is quoted so the shell passes `*.conf` unchanged to `find`.

The error report is a content search in one named current log:

```bash
grep -n '^ERROR' "$source_dir/var/log/operations.log"
```

`grep` reads the file's lines. `^ERROR` requires uppercase `ERROR` at the beginning of a line, and `-n` adds the source line number. Because only `operations.log` is named, the archived entry is out of scope by design.

### 3. Pipelines answer one question stage by stage

The category report uses:

```bash
cut -d ',' -f 2 "$source_dir/var/lib/inventory/assets.csv" | sort | uniq
```

- `cut` selects field 2 using comma as delimiter;
- `sort` groups equal categories;
- `uniq` keeps one line from each adjacent group.

The summary count pipelines use `wc -l` only after a producer has emitted exactly the records to count:

```bash
find "$source_dir" -type f | wc -l
find "$source_dir" -type f -name '*.conf' | wc -l
grep '^ERROR' "$source_dir/var/log/operations.log" | wc -l
```

Thus `wc -l` counts, respectively, all source-file paths, config-file paths, and current uppercase error lines.

### 4. Redirection makes generated reports reproducible

Each detailed report ends in `>`:

```bash
... > "$report_dir/config-files.txt"
```

That operator creates or replaces a disposable generated destination. It never writes to the named source.

The summary begins by replacing old content:

```bash
printf 'Field Report\n' > "$report_dir/summary.txt"
```

The remaining labels and counts use `>>`, but only after that fresh replacement within the same script run. A second unchanged run therefore does not accumulate an old summary; it reconstructs the same four lines.

### 5. Limited Bash remains reviewable

The script intentionally uses only material already taught:

- `#!/bin/bash`;
- one useful comment;
- two simple variable assignments;
- quoted variable expansions;
- `mkdir -p` for one fixed output directory;
- ordinary commands and short pipelines;
- `>` and bounded `>>` redirection.

It does not use arguments, conditions, loops, functions, command substitution, recursive `grep`, `find` actions, `eval`, privilege changes, or network access. Every source and destination is visible before execution.

## Required project brief

The completed structure is:

```text
field-report/
├── README.md
├── report.sh                 (mode 0744)
└── reports/
    ├── categories.txt
    ├── config-files.txt
    ├── errors.txt
    └── summary.txt
```

`README.md` is exactly:

```markdown
# Field Report

Run from this directory:
./report.sh

The script reads ../fixture/system and replaces four organized files in reports/.
```

`report.sh` is exactly:

```bash
#!/bin/bash
# Build deterministic reports from the supplied safe fixture.
source_dir='../fixture/system'
report_dir='reports'
mkdir -p "$report_dir"
find "$source_dir" -type f -name '*.conf' | sort > "$report_dir/config-files.txt"
grep -n '^ERROR' "$source_dir/var/log/operations.log" > "$report_dir/errors.txt"
cut -d ',' -f 2 "$source_dir/var/lib/inventory/assets.csv" | sort | uniq > "$report_dir/categories.txt"
printf 'Field Report\n' > "$report_dir/summary.txt"
printf 'Source files: ' >> "$report_dir/summary.txt"
find "$source_dir" -type f | wc -l >> "$report_dir/summary.txt"
printf 'Config files: ' >> "$report_dir/summary.txt"
find "$source_dir" -type f -name '*.conf' | wc -l >> "$report_dir/summary.txt"
printf 'Error entries: ' >> "$report_dir/summary.txt"
grep '^ERROR' "$source_dir/var/log/operations.log" | wc -l >> "$report_dir/summary.txt"
```

The generated outputs are:

`reports/config-files.txt`:

```text
../fixture/system/etc/app/main.conf
../fixture/system/etc/app/worker.conf
../fixture/system/etc/network.conf
```

`reports/errors.txt`:

```text
2:ERROR fan stalled
4:ERROR backup failed
```

`reports/categories.txt`:

```text
equipment
navigation
provisions
```

`reports/summary.txt`:

```text
Field Report
Source files: 8
Config files: 3
Error entries: 2
```

## Staged activity sequence

The activity has five deterministic stages.

### Stage 1 — Survey the source

The student navigates from `capstone-lab` to `fixture/system`, confirms location with `pwd`, and runs:

```bash
find . -type f | sort
```

This reinforces relative navigation, bounded search scope, file type, and deterministic ordering before any project is created.

### Stage 2 — Create the project brief

The student returns with `cd ../..`, creates and enters `field-report`, writes the exact README, and inspects it. At this point there is no script and no report directory.

### Stage 3 — Build and inspect the script

The student writes the exact fifteen-line script with the supplied `printf` construction and reads it with `cat`. The script remains mode `0644`; no report exists because writing script text is not script execution.

Before proceeding, the student must account for every line, source scope, pipeline stage, and destination.

### Stage 4 — Permission and run

The student runs:

```bash
chmod u+x report.sh
./report.sh
```

`chmod` changes mode only. Direct execution invokes the Bash shebang and generates four reports under `reports/`. Exact script bytes, exact mode, report types and contents, README content, source preservation, and current directory are checked.

### Stage 5 — Inspect and defend

The student reads all reports in a fixed order:

```bash
cat reports/config-files.txt reports/errors.txt reports/categories.txt reports/summary.txt
```

The final explanation connects each output block to its producer and accounts for every inclusion, exclusion, ordering choice, and count.

## Safety and craftsmanship

Require these habits throughout:

- state the current directory before resolving a relative path;
- use the smallest named source scope;
- inspect script text before granting execute permission;
- keep variables and wildcard/pattern arguments quoted as required;
- predict every write destination and whether `>` replaces or `>>` extends it;
- write only beneath `field-report`;
- treat the fixture as immutable source evidence;
- add only user execute permission with `chmod u+x`;
- correct the script and rerun it rather than hand-editing generated output;
- never use `sudo`, real system paths, removal commands, downloads, or broad search scopes.

Automation deserves stricter review than an isolated command because one mistake can repeat. The narrow script makes its complete read and write set visible to a beginning reviewer.

## Mastery check

A parent or tutor should require the student to answer these in his own words:

1. How do you know `fixture/system` is not the computer's real root filesystem?
2. From `field-report`, how does `../fixture/system` resolve one component at a time?
3. Why must the script run from the directory named in the README?
4. What does each test in `find "$source_dir" -type f -name '*.conf'` contribute?
5. Why is `*.conf` quoted?
6. What different questions do `find` and `grep` answer?
7. Why does `grep -n '^ERROR'` include exactly two lines and exclude the lowercase and archived decoys?
8. What does each stage of `cut | sort | uniq` receive and emit?
9. Why must `sort` precede `uniq` here?
10. What records reach each of the three `wc -l` commands?
11. Which streams travel through each pipe, and which final output is redirected?
12. Why is `>` appropriate for generated reports?
13. Why does using `>>` in the summary not accumulate old runs?
14. Why do writing, reading, and permissioning `report.sh` not create reports?
15. Which paths may change, and which must remain untouched?
16. Why will an unchanged second run produce the same report bytes?
17. If a generated report is wrong, why should the script be fixed instead of editing that report directly?

## Tutor boundaries

Keep support inside the exact fixture and project. Ask for location, prediction, and a left-to-right pipeline trace before commands. Give graduated hints in order: levels 1 and 2 should preserve planning; level 3 may reveal the exact current-stage command.

Do not introduce real-system inspection, absolute paths, `sudo`, `rm`, `mv`, `cp`, `find -delete`, `find -exec`, recursive `grep`, `xargs`, `sed`, `awk`, Python, command or process substitution, `eval`, functions, arrays, loops, conditions, arguments, background work, downloads, or network access. Do not accept hand-edited generated reports as equivalent to script output. Reset after source mutation, out-of-bounds work, alternate files, broad permissions, or changed required project text.

## Assessment and contract limits

The deterministic activity checks:

- the initial sorted fixture inventory and working directory;
- absence and later type of the project paths;
- exact README and script bytes;
- nonexecutable then exact executable mode;
- direct script execution with no arguments;
- exact bytes of all four generated reports;
- combined final inspection output;
- representative immutable source files and the archived decoy;
- final working directory.

These checks strongly establish the intended final state and some required commands. Primer's v1 contract observes only the latest command, not complete command history, and cannot directly score an oral explanation or prove every internal process that ran merely from final output. Exact script bytes provide deterministic evidence of the bounded searches, pipelines, quoting, and redirection; exact outputs demonstrate the expected result. A parent or tutor must still require the oral defense before granting full capstone mastery.

Lesson 20 will focus on clean-run verification, defect correction, and fuller explanation. This lesson's responsibility is the disciplined build.
