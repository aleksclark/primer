# Lesson 11: Find files by name

## Objectives

By the end of this lesson, the student can:

- read a `find` command as a starting scope followed by tests;
- choose `.` for the current tree or a narrower named subtree when the question permits it;
- use `-name 'NAME'` for an exact, case-sensitive basename;
- use a quoted wildcard pattern such as `-name '*.csv'`;
- use `-type f` for regular files and `-type d` for directories;
- predict which paths are included and excluded by scope, name, case, and type;
- interpret the paths printed by `find`; and
- keep searches read-only, bounded to the assigned workspace, and free of action predicates.

## Standards

- **Primary — `PRIMER.DL.6.FILES.2`:** Inspect files with safe read-only tools and confirm expected paths. The student locates exact files and file classes with deterministic `find` commands while preservation checks confirm that source content remains unchanged.
- **Reinforcement — `PRIMER.DL.6.NAV.2`:** Distinguish files from directories and follow relative paths inside a workspace. The student chooses relative starting points, reads nested result paths, and uses `-type` to distinguish same-named files and directories.

## Key idea: scope comes first

The general lesson form is:

```text
find STARTING-POINT TESTS...
```

Read it from left to right:

1. **Where may the search go?** The starting point defines the search tree.
2. **Which paths count as matches?** Tests such as `-name` and `-type` filter the paths encountered.
3. **What happens to matches?** In this lesson, plain `find` prints them. No modifying action is used.

For example:

```console
$ find projects -type f -name 'telemetry.log'
projects/orion/telemetry.log
projects/atlas/telemetry.log
```

This means: “Begin at `projects`, descend recursively, and print paths that are regular files whose basename is exactly `telemetry.log`.” A matching `archive/telemetry.log` cannot appear because `archive` is not beneath the starting point.

Search scope is both a correctness decision and a safety habit. Starting at the smallest directory that contains all possible answers:

- avoids irrelevant output;
- avoids inaccessible or sensitive areas;
- reduces work;
- makes the result easier to check; and
- makes intent visible to another reader.

Do not search `/` merely because it is broad. The root directory can include the entire mounted system, virtual filesystems, permission boundaries, and data unrelated to the question. Broad is not the same as thorough or correct.

## Starting points

### Current tree: `.`

A dot means the current directory. With the activity starting in `workspace/search-lab`:

```console
$ find . -name 'mission.txt'
./projects/orion/notes/mission.txt
./archive/mission.txt
```

`find` examines `.` itself and then recursively descends into directories below it. Printed paths retain the starting-point form, so results beginning at `.` begin with `./`.

Use `.` only when the entire current tree is relevant. If the question says “only under projects,” choose `projects` instead.

### Named relative subtree

A relative directory such as `projects` or `data` limits descent to that subtree:

```console
$ find data -type f -name '*.csv'
data/current/readings.csv
data/raw/sensors.csv
```

The command does not inspect sibling trees such as `projects` or `archive`. This reinforces path literacy: the starting point is interpreted relative to the current working directory.

## Name tests

### Exact names

`-name PATTERN` tests the final component of each path—the **basename**. It does not compare the whole printed path.

```console
$ find . -name 'mission.txt'
```

This can match both `./archive/mission.txt` and `./projects/orion/notes/mission.txt` because both basenames are `mission.txt`. It does not require their parent directories to have the same names.

Plain `-name` is case-sensitive:

- `mission.txt` matches `mission.txt`;
- `mission.txt` does not match `Mission.txt`;
- `*.csv` matches `sensors.csv`;
- `*.csv` does not match `SENSORS.CSV`.

Case is part of a Linux filename. Do not silently broaden the search when exact capitalization is requested.

### Wildcard patterns and quoting

The wildcard `*` means “zero or more characters” within a `find -name` pattern. Thus:

```text
*.csv
```

matches basenames ending in lowercase `.csv`.

Quote patterns containing wildcards:

```console
$ find data -type f -name '*.csv'
```

The quotes instruct the shell to pass the literal argument `*.csv` to `find`. Then `find` applies the pattern separately to basenames it encounters during recursion.

Without quotes, the shell tries to expand `*.csv` in the current directory before `find` runs. Depending on the current directory, it might remain unchanged, expand to one name, or expand to several arguments. A command that happens to work only because no local filename triggers expansion is fragile. Quoting states who should interpret the wildcard: `find`, not the shell.

The command checker observes the executed argument as `*.csv`; quote characters are shell syntax and are removed before the program receives its arguments. The lesson still requires the student to type quotes for robust shell behavior.

## Type tests

Names do not prove path type. A regular file can be named `reports`, and a directory can be named `reports`. An extension-like suffix also does not prove that a path is a regular file.

Use:

- `-type f` — match regular files;
- `-type d` — match directories.

Example:

```console
$ find projects -type d -name 'reports'
projects/orion/reports
```

The fixture also contains `projects/atlas/reports`, but that path is a regular file. It passes the name test and fails the directory-type test, so it is not printed.

Likewise, the data tree contains a directory named `readings.csv.d`. Its name does not end exactly in `.csv`, and `-type f` would exclude it even if a directory had a matching suffix.

Each path must pass **every** test in these lesson commands. In:

```text
find data -type f -name '*.csv'
```

a path must be beneath `data`, be a regular file, and have a basename matching the lowercase pattern.

## Safe searches

Plain `find` with tests is read-only in this lesson: it walks directory entries, inspects path metadata, and prints matches. The `find` program also supports actions that can run commands or remove paths. Those capabilities are deliberately outside this lesson.

Never add these here:

- `-delete` — removes matched paths;
- `-exec` or `-execdir` — runs a command using matches;
- `-ok` — interactively proposes running commands;
- a pipe to another modifying command;
- `xargs` that turns printed names into command operands.

Safety rules:

1. Begin inside the assigned workspace.
2. State the intended search boundary before typing.
3. Choose the smallest starting point containing all possible answers.
4. Add only read-only tests: `-name`, `-type f`, and `-type d` in this lesson.
5. Quote wildcard patterns.
6. Predict likely matches and deliberate decoys.
7. Read output before drawing a conclusion.
8. Treat an empty successful result as “no path in this scope passed every test,” not automatically as an error.
9. Reset if any fixture is changed.

## Vocabulary

- **`find`** — a program that recursively walks one or more starting paths, evaluates an expression for each encountered path, and normally prints matches in the forms used here.
- **starting point** — the file or directory where `find` begins; it sets the search scope.
- **scope** — the bounded portion of the filesystem a search may inspect.
- **recursive** — descending through directories and their nested descendants rather than examining only one level.
- **test / predicate** — a condition evaluated for a path, such as `-name` or `-type`.
- **basename** — the final component of a path, such as `mission.txt` in `archive/mission.txt`.
- **pattern** — text containing wildcard syntax used to test names.
- **wildcard** — a pattern character such as `*`; here `*` represents zero or more characters.
- **quote** — shell syntax that prevents wildcard expansion before an argument reaches `find`.
- **regular file (`-type f`)** — an ordinary file containing bytes, as opposed to a directory or another path type.
- **directory (`-type d`)** — a path that maps entry names to other filesystem objects.
- **case-sensitive** — treating uppercase and lowercase letters as different.
- **match** — a path that passes every test in the expression.
- **action** — an operation performed for a path; modifying actions are excluded from this lesson.
- **fixture** — resettable supplied data designed to give known results and decoys.

## Fixture map

The activity opens in `workspace/search-lab`:

```text
search-lab/
├── archive/
│   ├── mission.txt
│   ├── telemetry.log
│   └── old/
│       └── summary.txt
├── data/
│   ├── current/
│   │   ├── readings.csv
│   │   └── readings.csv.d/
│   │       └── part.txt
│   └── raw/
│       ├── SENSORS.CSV
│       └── sensors.csv
├── loose.txt
└── projects/
    ├── atlas/
    │   ├── reports                 regular file
    │   └── telemetry.log
    └── orion/
        ├── notes/
        │   ├── Mission.txt
        │   └── mission.txt
        ├── reports/                directory
        │   └── summary.txt
        └── telemetry.log
```

The fixture is intentionally rich:

- duplicate basenames in different subtrees test recursive scope;
- `mission.txt` and `Mission.txt` test case sensitivity;
- logs inside and outside `projects` test scope exclusion;
- same-named file and directory paths test `-type`;
- lowercase and uppercase CSV names test pattern case;
- an extension-like directory provides a type and naming decoy;
- unrelated text and summary files discourage overgeneralization.

All supplied files are mode `0444`, while directories are `0755`. Searches must preserve every path, type, mode, and content.

## Guided practice

Complete four tasks in order. Before every command, say:

> “I will start at ___ because ___. A matching path must be type ___ and its basename must ___. I predict ___ will match and ___ will not.”

### 1. Exact name in the whole lab

Question: Where are all paths whose basename is exactly lowercase `mission.txt`?

```console
$ find . -name 'mission.txt'
./projects/orion/notes/mission.txt
./archive/mission.txt
```

Interpretation:

- `.` includes all descendants of the current lab;
- both printed paths have the requested basename;
- `Mission.txt` is excluded because capitalization differs;
- no type test is needed for this controlled question, though both expected matches are regular files.

### 2. Narrow scope to projects

Question: Which regular files named `telemetry.log` are under `projects`?

```console
$ find projects -type f -name 'telemetry.log'
projects/orion/telemetry.log
projects/atlas/telemetry.log
```

Interpretation:

- `projects` is the smallest named subtree containing every possible answer;
- `archive/telemetry.log` matches the name and type tests but is never visited, so it cannot appear;
- `-type f` makes the requested path type explicit.

### 3. Find only directories

Question: Which paths named `reports` under `projects` are directories?

```console
$ find projects -type d -name 'reports'
projects/orion/reports
```

Interpretation:

- `projects/orion/reports` passes both tests;
- `projects/atlas/reports` passes `-name` but fails `-type d` because it is a regular file;
- a familiar-looking name is not evidence of type.

### 4. Match lowercase CSV files

Question: Which regular files under `data` have names ending in lowercase `.csv`?

```console
$ find data -type f -name '*.csv'
data/current/readings.csv
data/raw/sensors.csv
```

Interpretation:

- quotes preserve `*.csv` for `find`;
- recursion reaches both `current` and `raw`;
- `SENSORS.CSV` fails the case-sensitive name test;
- `readings.csv.d` does not end in `.csv` and is a directory;
- the result contains only regular files in the requested scope.

## Mastery evidence

The activity has four ordered tasks with deterministic command, output, path, and preservation evidence:

1. Exact observation of `find . -name mission.txt`, an anchored output check accepting exactly the two expected paths in either traversal order, preserved matching files, and cwd prove a whole-lab exact-name search.
2. Exact observation of `find projects -type f -name telemetry.log`, an anchored output check accepting exactly the two project paths in either traversal order, preservation of the outside-scope log, and cwd prove deliberate narrowing and regular-file filtering.
3. Exact observation of `find projects -type d -name reports`, one exact output line, and independent `path_type` checks prove discrimination between a same-named directory and regular file.
4. Exact observation of `find data -type f -name *.csv`, an anchored output check accepting exactly the two lowercase CSV paths in either traversal order, content preservation checks, and final cwd prove wildcard name matching and bounded file search.

`command_properties` records the executable and arguments received after shell parsing. Therefore it proves that `find` received the literal wildcard argument `*.csv`, but it cannot alone prove whether the student typed quotes. The exact task instructions and tutor review require the robust quoted form. `pipeline_output` observes the latest standard output, while fixture checks independently prove source state.

The verifier cannot directly grade the student's explanation of why a path was excluded. A parent or tutor must require a precise account of starting scope, recursion, basename, case, type, and quoting before granting conceptual mastery.

The output checks are deterministic about membership while allowing either order for two-result searches, because plain `find` does not promise a portable sorting order. The commands use no sorting or pipes because those topics belong to later lessons.

## Parent or tutor review

Ask the student to answer without running additional commands:

1. In the general form `find STARTING-POINT TESTS`, what job does each part perform?
2. What does `.` mean in this activity?
3. Why do results from a `.` starting point begin with `./`, while results from `projects` begin with `projects/`?
4. Does `find projects` examine `archive`? Why not?
5. Why is the smallest sufficient starting point safer and clearer than `/`?
6. Is `-name` comparing the full printed path or its basename?
7. Why does `-name 'mission.txt'` exclude `Mission.txt`?
8. What does `*` mean in `'*.csv'`?
9. Why are the quotes important even if an unquoted command appears to work once?
10. Who should interpret the wildcard in this lesson: the shell or `find`?
11. What does `-type f` mean? What does `-type d` mean?
12. Why does `projects/atlas/reports` fail the directory search?
13. Why does `archive/telemetry.log` fail to appear in the projects search?
14. Why does `SENSORS.CSV` fail the CSV search?
15. Does a `.csv`-looking name prove that a path is a regular file?
16. What can an empty result mean when `find` exits successfully?
17. Why are `-delete` and `-exec` excluded from a safe introductory search?
18. Which checks show that searching did not alter the fixture?

Require complete explanations, not just command recitation.

## Misconceptions

- **“`find` searches the entire computer automatically.”** It searches only the supplied starting point or points and descendants it can traverse.
- **“The starting point is just where output labels begin.”** It determines which filesystem tree is visited at all.
- **“Starting at `/` is the safest way not to miss anything.”** It exceeds the question's boundary, creates noise, and may inspect inaccessible or sensitive areas.
- **“I must `cd` into every directory first.”** `find` descends recursively from its starting point.
- **“`-name` examines the complete path.”** It tests the basename. More advanced whole-path predicates are outside this lesson.
- **“Linux ignores capitalization in filenames.”** The names in this fixture are case-sensitive.
- **“Quotes become part of the pattern.”** The shell removes quote syntax and passes the protected text as one argument.
- **“If an unquoted wildcard works, quoting is unnecessary.”** Shell expansion depends on current-directory contents and can change unexpectedly.
- **“An extension tells me the type.”** Names are conventions. Use `-type` when the question specifies a filesystem type.
- **“`-type f` means filename.”** It means regular file; `f` does not refer to the spelling of a name.
- **“Every test prints a separate set of results.”** In these commands, each path must pass all tests to be printed.
- **“No output means the program is broken.”** It may mean no path inside the chosen scope passed every test.
- **“`find` is always harmless.”** Plain tests are read-only here, but action predicates can execute programs or delete matches.

## Tutor strategy and constraints

Use a consistent graduated sequence:

1. Ask the student to mark the boundary on the fixture map.
2. Ask for the smallest relative starting point containing all possible answers.
3. Ask whether the target must be a regular file, a directory, or either.
4. Ask whether the name is exact or a pattern and whether case matters.
5. For a wildcard, ask what would happen if the shell expanded it before `find` ran.
6. Require a prediction naming both expected matches and specific decoys.
7. Reveal the exact command only at hint level 3.
8. After execution, require the student to explain every included and excluded path.

Keep command coaching strictly within these forms:

```text
find . -name 'mission.txt'
find projects -type f -name 'telemetry.log'
find projects -type d -name 'reports'
find data -type f -name '*.csv'
```

Do not introduce `/`, home searches, parent paths, `-iname`, `-path`, `-maxdepth`, `-mindepth`, multiple roots, logical operators, parentheses, `-print0`, formatting actions, pipes, sorting, redirection, command substitution, loops, or aliases. Never suggest `-delete`, `-exec`, `-execdir`, `-ok`, or `xargs`.

Do not accept repeated `ls`, manual traversal, shell globbing alone, or `grep` as equivalent evidence. If output differs, inspect current working directory, starting point, exact case, quote use, test spelling, and path type. If any fixture changes, reset rather than repairing it with more commands.

## Safety and craftsmanship

- Remain in `workspace/search-lab`.
- State scope before syntax.
- Use `.` only for the whole supplied lab.
- Prefer `projects` or `data` when the question names those subtrees.
- Never use `/`, `..`, or a workspace ancestor as a starting point in this activity.
- Use only `-name`, `-type f`, and `-type d` tests.
- Quote every wildcard pattern.
- Use exact lowercase spelling when the question is case-sensitive.
- Predict included and excluded paths before running the search.
- Read the complete output and relate each path to the starting point.
- Do not add actions or connect output to another command.
- Preserve all fixture paths, types, modes, and contents.
- Reset after any accidental modification.

## Extension

Answer verbally or on paper; do not run commands in the assessed fixture.

For each request, give a safe `find` command and explain scope, type, name test, quoting, and at least one exclusion:

1. Under `archive` only, find regular files named `summary.txt`.
2. In the whole lab, find directories named `reports`.
3. Under `projects`, find regular files ending in `.log`.
4. Under `data/current`, find directories whose names end in `.d`.
5. In the whole lab, find regular files named exactly `Mission.txt`.
6. Explain why `find / -name 'mission.txt'` is a poor answer to a question limited to this lab.
7. Explain what could happen if `*.log` is left unquoted in a directory that already contains several matching names.
8. Explain the difference between a successful empty result and a command error.

Then compare these pairs without executing them:

```text
find . -name 'reports'
find . -type d -name 'reports'
```

```text
find . -name '*.csv'
find data -type f -name '*.csv'
```

For each pair, identify which command is broader, which more precisely answers a typed and scoped question, and which decoys could appear in the broader result.
