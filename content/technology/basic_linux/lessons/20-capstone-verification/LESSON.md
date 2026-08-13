# Lesson 20: Capstone verification and explanation

## Purpose

This final lesson begins with a fresh, deliberately imperfect `field-report` project. It is not the student's Lesson 19 workspace: Primer activities are isolated, so every required source file, defect, stale report, and clean test datum is supplied again. The student must inspect the inherited work, diagnose defects from evidence, repair the source rather than patch generated output, run the project from its documented location, preserve standard error separately, and leave a concise verification record.

The finished project must contain:

- an executable `field-report/report.sh`;
- `field-report/README.md`, explaining where to run the script, what it reads and writes, and why errors are separate;
- six script-generated evidence files under `field-report/reports/`;
- a saved first-run summary and an empty repeatability diff under `field-report/verification/`;
- `field-report/VERIFICATION.md`, explaining the evidence and the important filesystem, pipeline, permission, and scripting decisions.

The goal is not to replace bad text with supplied good text mechanically. The student must connect each observed symptom to a cause, predict the effect of each repair, and explain why the final evidence supports the claim that the project works.

## Objectives

By the end of this lesson, the student can:

- orient within a newly supplied workspace with `pwd`, `ls`, and relative paths;
- distinguish immutable clean data, editable project source, generated reports, and verification evidence;
- inspect an inherited README, script, and stale report before changing anything;
- identify wrong relative-path resolution, missing quotes, overly broad `grep`, unstable ordering, misuse of `uniq`, cumulative `>>`, missing error separation, and missing execute permission;
- repair a bounded Bash script and documentation from explicit acceptance criteria;
- grant only user execute permission and run the script directly from its documented directory;
- keep standard error in a dedicated report while preserving successful report data;
- test repeatability by saving evidence from one clean run, running again, and comparing the two summaries with `diff`;
- write a verification note that makes claims, cites file evidence, and explains why the evidence is relevant;
- defend every command and reject hand-edited generated output as false provenance.

## Standards

- **Primary — `PRIMER.DL.6.FILES.1`:** Repair and organize a small project with source, generated reports, and verification evidence at explicit paths.
- **Primary — `PRIMER.DL.6.FILES.2`:** Inspect inherited and generated text, compare repeated output, and verify exact contents with read-only tools.
- **Reinforcement — `PRIMER.DL.6.NAV.1`:** Use `pwd`, `ls`, and `cd` to orient in the isolated verification workspace.
- **Reinforcement — `PRIMER.DL.6.NAV.2`:** Resolve `../clean-data/system` from `field-report` and distinguish source directories from files and generated destinations.

Scripting-specific standards are still pending, so the activity records `standardsStatus: pending-scripting-standards` while using the closest seeded digital-literacy standards.

## Supplied isolated fixture

The initial working directory is `workspace/verification-lab`:

```text
verification-lab/
├── clean-data/
│   └── system/
│       ├── etc/
│       │   ├── app/main.conf
│       │   ├── app/worker.conf
│       │   └── network.conf
│       ├── home/operator/notes.txt
│       ├── usr/share/docs/runbook.txt
│       └── var/
│           ├── lib/inventory/assets.csv
│           └── log/
│               ├── archive.log
│               └── operations.log
└── field-report/
    ├── README.md              (inaccurate and incomplete)
    ├── report.sh              (nonexecutable and defective)
    └── reports/
        └── summary.txt        (stale, false evidence)
```

`clean-data/system` is synthetic lesson data, not the machine's `/`. Its eight files are mode `0444` and must remain unchanged. The fixture deliberately contains:

- three lowercase `.conf` regular files;
- eight regular files total;
- two uppercase `ERROR` lines in `operations.log`;
- one lowercase `error` decoy in `operations.log`;
- one uppercase archived error outside the requested current-log scope;
- repeated, nonadjacent categories in `assets.csv`.

The inherited project is deliberately wrong:

1. Its README tells the operator to run from the wrong directory and does not explain error evidence.
2. `report.sh` is mode `0644`, so it is not directly executable.
3. `source_dir='clean-data/system'` is wrong when the script runs from `field-report`; it needs one parent step.
4. Several variable expansions are unquoted.
5. The config inventory is unsorted.
6. Error search uses a broad case-insensitive pattern over both logs, admitting lowercase and archived decoys.
7. `uniq` is used without sorting the nonadjacent duplicate categories first.
8. The summary uses only `>>`, so stale and repeated content accumulates.
9. Diagnostic standard error is merged into report data instead of being preserved separately.
10. The stale `reports/summary.txt` claims 99 source files and is not trustworthy merely because it exists.

These defects are evidence to reason from, not a list to fix blindly. For every repair, the student should be able to state the observed symptom, its cause, the changed line, and the expected evidence after rerunning.

## Verification model

A defensible verification separates four things:

1. **Source data** — immutable inputs under `clean-data/system`.
2. **Project source** — human-authored `README.md`, `report.sh`, and `VERIFICATION.md`.
3. **Generated report evidence** — files recreated by `report.sh` under `reports/`.
4. **Repeatability evidence** — a saved first-run summary and the empty output of comparing it with the second-run summary.

A generated file is not trustworthy just because its name looks right. Its provenance matters: repair `report.sh`, run it, inspect its outputs, and compare repeated runs. Never hand-edit a generated report to satisfy an expected value.

## Required corrected design

### Run location and paths

The README must direct the operator to enter `field-report` and run:

```bash
./report.sh
```

From that directory:

```bash
source_dir='../clean-data/system'
report_dir='reports'
```

The `..` leaves `field-report` for `verification-lab`; `clean-data/system` then descends into the supplied input tree. `reports` resolves inside the project. Every variable expansion used as a path is double-quoted.

### Bounded searches and pipelines

The corrected config inventory is:

```bash
find "$source_dir" -type f -name '*.conf' | sort
```

The search starts only at the synthetic source, keeps regular files with lowercase `.conf` names, and sorts paths so traversal order cannot change the report.

The corrected current-error extract is:

```bash
grep -n '^ERROR' "$source_dir/var/log/operations.log"
```

The one named current log excludes the archive. The anchored, case-sensitive pattern excludes the lowercase decoy. `-n` preserves useful line-number evidence.

The corrected category pipeline is:

```bash
cut -d ',' -f 2 "$source_dir/var/lib/inventory/assets.csv" | sort | uniq
```

`cut` selects the category field, `sort` groups equal categories, and `uniq` removes adjacent duplicates. Reversing or omitting those stages changes the meaning.

### Replacement and standard-error separation

At the start of every run, the script creates `reports/` if needed and replaces `reports/run-errors.txt` with an empty file:

```bash
: > "$report_dir/run-errors.txt"
```

The report-building commands are grouped and use:

```bash
} 2>> "$report_dir/run-errors.txt"
```

Only standard error from the group is appended to the freshly emptied diagnostic file. Normal report data remains in its named files. With the valid clean fixture, `run-errors.txt` must be empty; if a report command fails, its diagnostic has a dedicated destination instead of contaminating successful output.

Every generated data report uses `>` or begins with a replacing write before bounded `>>` operations. The script also replaces `reports/run-output.txt` with one completion line. Therefore stale evidence is discarded and a second unchanged run reconstructs identical bytes.

### Permission

The repaired script must be mode `0744`, reached with:

```bash
chmod u+x report.sh
```

This adds execute permission only for the owner. It does not execute the script, prove correctness, or create reports.

## Canonical completed project

The required structure is:

```text
field-report/
├── README.md
├── report.sh                  (mode 0744)
├── VERIFICATION.md
├── reports/
│   ├── categories.txt
│   ├── config-files.txt
│   ├── errors.txt
│   ├── run-errors.txt         (empty after a successful clean run)
│   ├── run-output.txt
│   └── summary.txt
└── verification/
    ├── repeatability.diff     (empty)
    └── summary-first.txt
```

`README.md`:

```markdown
# Field Report

Run from this directory:
./report.sh

The script reads immutable data from ../clean-data/system and replaces six evidence files in reports/.
Normal report data stays in its named files; command diagnostics are kept separately in reports/run-errors.txt.
```

`report.sh`:

```bash
#!/bin/bash
# Rebuild deterministic evidence from the supplied clean fixture.
source_dir='../clean-data/system'
report_dir='reports'
mkdir -p "$report_dir"
: > "$report_dir/run-errors.txt"
{
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
  printf 'Field report complete\n' > "$report_dir/run-output.txt"
} 2>> "$report_dir/run-errors.txt"
```

The expected generated evidence is:

`reports/config-files.txt`:

```text
../clean-data/system/etc/app/main.conf
../clean-data/system/etc/app/worker.conf
../clean-data/system/etc/network.conf
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

`reports/summary.txt` and `verification/summary-first.txt`:

```text
Field Report
Source files: 8
Config files: 3
Error entries: 2
```

`reports/run-output.txt`:

```text
Field report complete
```

Both `reports/run-errors.txt` and `verification/repeatability.diff` are empty.

`VERIFICATION.md`:

```markdown
# Verification Note

- Clean source: I used only ../clean-data/system and left its eight supplied files unchanged.
- Run evidence: reports/summary.txt records 8 source files, 3 configuration files, and 2 current ERROR entries.
- Error separation: normal evidence has named report files; reports/run-errors.txt is separately captured and empty for the successful run.
- Repeatability: I saved the first summary, ran ./report.sh again, and diff produced an empty verification/repeatability.diff.
- Explanation: find selects bounded paths, grep selects anchored lines in one current log, and cut | sort | uniq produces distinct ordered categories.
- Permission: mode 0744 lets the owner execute the reviewed script without granting group or other execute permission.
```

## Five-stage activity sequence

### 1. Inspect and diagnose before editing

From `verification-lab`, the student uses `pwd` and `ls`, then reads the inherited README, script, and stale summary. The student identifies why the stale number is not proof and maps each defect to a likely symptom. No file changes are allowed in this stage.

### 2. Repair project source and documentation

The student enters `field-report`, rewrites `README.md` and `report.sh` to meet the acceptance criteria, and inspects both. Generated reports are not hand-edited. Before execution, the student traces every relative path, pipeline, redirection, and possible error destination.

### 3. Permission and first clean run

The student adds only user execute permission, runs `./report.sh`, and inspects all six generated evidence files. The stale summary must be replaced, not extended. The student saves the first-run summary as `verification/summary-first.txt`.

### 4. Repeat and compare

Without changing input or script, the student runs `./report.sh` again and redirects `diff -u verification/summary-first.txt reports/summary.txt` to `verification/repeatability.diff`. An empty diff supports the narrow claim that the compared summaries have identical bytes across the two runs. It does not prove every possible input would work.

### 5. Write and defend the verification note

The student writes `VERIFICATION.md`, reads it with the summary and completion/error evidence, and explains each claim. The note must cite paths, distinguish stdout-like normal evidence from stderr diagnostics, explain the pipelines, account for exact counts and excluded decoys, and describe why mode `0744` and an empty diff matter.

## Mastery questions

A parent or tutor should require answers in the student's own words:

1. Why is this a fresh fixture rather than a continuation of Lesson 19?
2. Which paths are immutable input, editable source, generated output, and saved verification evidence?
3. Why was the inherited summary not trustworthy?
4. From `field-report`, how does `../clean-data/system` resolve one component at a time?
5. Why would `clean-data/system` be wrong from that same directory?
6. Why are path variable expansions quoted?
7. What question does each of `find`, `grep`, `cut`, `sort`, `uniq`, and `wc -l` answer?
8. Why does the corrected error report exclude both the archived uppercase line and the lowercase current-log line?
9. Why must `sort` come before `uniq` for the supplied CSV?
10. Why must generated files be replaced rather than repaired by hand?
11. What is standard error, and why does it have a separate destination here?
12. Why is an empty `run-errors.txt` useful evidence but not proof that the script can never fail?
13. What changes when `chmod u+x report.sh` runs, and what does not?
14. What exactly does the empty repeatability diff establish?
15. Why does testing twice add evidence beyond inspecting the script once?
16. Which claims in `VERIFICATION.md` are supported by which files?

## Safety and craftsmanship

Require the student to:

- remain under `workspace/verification-lab`;
- state the current directory before interpreting relative paths;
- treat `clean-data/system` as immutable;
- inspect inherited work before changing it;
- predict writes and stream destinations before execution;
- change project source, then regenerate evidence instead of editing reports;
- use only the named current log for current error evidence;
- use `chmod u+x`, never broad permission changes;
- keep standard error separate from normal report content;
- make claims no broader than the evidence supports;
- reset after source mutation, work outside the lab, or hand-edited generated evidence.

Never use `sudo`, real system paths, downloads, network access, removal commands, `find -delete`, `find -exec`, recursive `grep`, `xargs`, `eval`, background work, privilege changes, or writes outside `field-report`.

## Tutor boundaries

The tutor should coach diagnosis rather than dictate replacement text. Ask first: “What did you observe?”, “Which line could cause it?”, “What stream or file should contain the result?”, and “What evidence would distinguish the two explanations?” Hints progress from identifying a concept, to narrowing the line or command family, to describing the required structure. Even at level 3, do not provide a paste-ready complete script or verification note; send the student back to the acceptance criteria and require an explanation before accepting a correction.

Do not accept matching files without explanation. Do not accept hand-edited reports, a broadened search, merged diagnostics, `chmod 777`, or a verbal claim of repeatability without the saved summary and empty diff. Praise precise diagnosis and limited claims, not speed.

## Assessment and contract limits

Deterministic checks can establish exact final source bytes, script mode, exact generated report bytes, empty diagnostic and diff files, representative source preservation, final location, and a final read-only inspection command. Exact script bytes also expose the intended bounds, quoting, ordering, replacement, and stderr redirection.

Primer v1 observes only the latest command rather than full history. It cannot prove that the student personally reasoned from the initial defects, that every generated file came from the script rather than manual editing, that two runs occurred, or that the prose explanation is understood. The saved first summary, empty diff, exact final script, and generated outputs are strong deterministic evidence, but a parent or tutor must still conduct the oral defense before granting full capstone mastery.
