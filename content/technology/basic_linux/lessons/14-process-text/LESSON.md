# Lesson 14: Process text

## Objectives

By the end of this lesson, the student can:

- describe a structured text record as fields separated by a consistent delimiter;
- use `sort` to order complete lines without changing the source file;
- use `cut -d ',' -f N` to select one field from simple comma-separated records;
- use `tr 'a-z' 'A-Z'` to normalize lowercase text to uppercase;
- explain that `uniq` removes adjacent duplicate lines, not duplicates scattered throughout unsorted input;
- place `sort` before `uniq` when equal values must first be brought together;
- use `wc -l` at the end of a pipeline to count the lines produced by earlier stages;
- trace each short pipeline from left to right in plain language; and
- preserve the supplied fixture while answering questions through stdout.

## Standards

These references use the current custom standards while a more specific structured-text processing standard is pending.

- **Primary — `PRIMER.DL.6.FILES.2`:** Inspect file contents with safe read-only tools and confirm expected text. The student selects, orders, transforms, deduplicates, and counts data from a supplied structured text file without changing it.
- **Reinforcement — `PRIMER.DL.6.FILES.1`:** Organize a small workspace according to an explicit structure. The student works accurately within the assigned fixture and treats consistent field structure and ordering as forms of digital organization.

The activity records `standardsStatus: pending-new-standards` in metadata so this provisional alignment is explicit.

## Why text processing matters

Small text files often contain one record per line. If each record uses the same field order and delimiter, simple command-line tools can answer useful questions without opening an editor or writing a program.

This lesson uses a deliberately simple CSV file. **CSV** means comma-separated values. Every line has exactly three fields:

```text
item,category,zone
```

For example:

```text
rope,equipment,north
```

Here:

1. field 1 is `rope`;
2. field 2 is `equipment`; and
3. field 3 is `north`.

The comma is the **delimiter**: the character separating one field from the next.

Real CSV can be more complicated. A field may be quoted and may itself contain a comma. The basic `cut` commands in this lesson are suitable only because the fixture guarantees that every comma is a field boundary. Do not generalize this simple method to arbitrary CSV without first inspecting its format.

## One clear purpose per stage

A pipeline is easiest to understand when each stage does one visible job:

```text
input records | select | transform | order | deduplicate | count
```

Not every question needs every stage. Use only the stages required for the answer.

Before running a pipeline, trace it from left to right:

1. What text enters this stage?
2. What one change does this stage make?
3. What text leaves this stage?
4. Why does the next stage need that output?

This prevents a pipeline from becoming an opaque trick memorized as punctuation.

## `sort`: order lines

`sort` reads lines, orders them, and writes the ordered lines to stdout:

```console
$ sort supplies.csv
biscuits,provisions,east
compass,navigation,east
lantern,equipment,south
map,navigation,west
radio,equipment,west
rope,equipment,north
water,provisions,north
```

Plain `sort` compares complete lines. Since each fixture line begins with an item name, the item names determine this order.

In this lesson there is no redirection, so `sort` does not rewrite `supplies.csv`. It only prints an ordered view. The original file remains in its original order.

## `cut`: select a field

The general form used here is:

```text
cut -d ',' -f N file
```

- `-d ','` says that comma is the delimiter.
- `-f N` selects field number `N`.
- Fields are numbered starting at 1.

To select every category, use field 2:

```text
cut -d ',' -f 2 supplies.csv
```

The resulting stream is:

```text
navigation
equipment
navigation
provisions
equipment
provisions
equipment
```

`cut` processes each line independently. It selects text; it does not sort it, remove duplicates, or count it.

The quote marks around the comma are shell syntax that protect and clearly identify the delimiter argument. `cut` receives the comma itself.

## `tr`: translate characters

`tr` reads stdin and translates characters. This lesson uses:

```text
tr 'a-z' 'A-Z'
```

Each lowercase ASCII letter is replaced with its corresponding uppercase letter. For example, input containing:

```text
east
north
```

becomes:

```text
EAST
NORTH
```

`tr` does not choose a CSV field. That is why `cut` comes first when only the zone field should be transformed. It also does not order or deduplicate lines.

## `uniq`: collapse adjacent duplicates

`uniq` compares neighboring lines. Given:

```text
equipment
equipment
navigation
navigation
provisions
```

plain `uniq` produces:

```text
equipment
navigation
provisions
```

But given:

```text
equipment
navigation
equipment
```

plain `uniq` leaves all three lines because the equal lines are not adjacent.

That is why these lesson pipelines put `sort` before `uniq`:

```text
... | sort | uniq
```

The roles are distinct:

1. `sort` brings equal lines together.
2. `uniq` keeps one line from each adjacent group.

Do not memorize `sort | uniq` as a magical phrase. Explain why adjacency matters each time.

## `wc -l`: count final lines

`wc -l` counts newline-terminated lines. Its meaning depends on where it appears.

Used with a filename:

```text
wc -l supplies.csv
```

it would count source records. Used at the end of this pipeline:

```text
cut -d ',' -f 2 supplies.csv | sort | uniq | wc -l
```

it counts the three distinct category lines produced by the earlier stages.

Trace the count:

1. `cut` emits seven category lines, one per source record.
2. `sort` orders those seven lines and groups equal values.
3. `uniq` reduces the groups to three lines.
4. `wc -l` receives and counts those three lines.

The answer is therefore `3`, not `7`.

## Fixture map

The activity opens in `workspace/text-pipeline-lab`:

```text
text-pipeline-lab/
└── supplies.csv
```

`supplies.csv` is read-only and contains:

```text
map,navigation,west
rope,equipment,north
compass,navigation,east
water,provisions,north
lantern,equipment,south
biscuits,provisions,east
radio,equipment,west
```

Every record follows the same field structure:

| Field | Meaning | Example |
|---:|---|---|
| 1 | item | `rope` |
| 2 | category | `equipment` |
| 3 | zone | `north` |

The file must remain byte-for-byte unchanged. No output files are needed; every answer appears on stdout.

## Guided practice

Complete four tasks in order. Before each command, state:

> “Stage 1 reads ___ and emits ___. Stage 2 receives ___ and emits ___. The final stdout should be ___.”

Add stages to that explanation as needed.

### 1. Sort complete records

Run:

```console
$ sort supplies.csv
biscuits,provisions,east
compass,navigation,east
lantern,equipment,south
map,navigation,west
radio,equipment,west
rope,equipment,north
water,provisions,north
```

Explain that:

- `sort` reads complete lines;
- the first field controls this particular order because it begins each line;
- all three fields stay together; and
- the file itself retains its supplied order.

### 2. List each category once

Run:

```console
$ cut -d ',' -f 2 supplies.csv | sort | uniq
equipment
navigation
provisions
```

Trace it:

1. `cut` selects field 2 and emits seven category values.
2. `sort` groups the equal category lines.
3. `uniq` emits one line for each adjacent group.

The pipeline has no counting stage, so it prints the category names themselves.

### 3. Normalize and list zones

Run:

```console
$ cut -d ',' -f 3 supplies.csv | tr 'a-z' 'A-Z' | sort | uniq
EAST
NORTH
SOUTH
WEST
```

Trace it:

1. `cut` selects field 3.
2. `tr` changes lowercase zone letters to uppercase.
3. `sort` orders and groups the uppercase values.
4. `uniq` keeps one copy of each adjacent value.

The order matters for understanding: the pipeline transforms the selected field before ordering the transformed lines.

### 4. Count distinct categories

Run:

```console
$ cut -d ',' -f 2 supplies.csv | sort | uniq | wc -l
3
```

Explain that `wc -l` sees three lines after deduplication. It does not inspect the original file itself and does not know what a category means; it simply counts the lines passed to it.

## Common mistakes and corrections

### Treating fields as zero-based

Incorrect idea: field 2 is the third field because counting begins at zero.

Correction: `cut -f` field numbering begins at 1. Category is field 2 and zone is field 3.

### Expecting `uniq` to search globally

Incorrect idea: `uniq` removes every duplicate no matter where it occurs.

Correction: `uniq` compares adjacent lines. Use `sort` first here so equal values become neighbors.

### Expecting `sort` to edit the file

Incorrect idea: `sort supplies.csv` permanently rearranges the source.

Correction: plain `sort` writes an ordered view to stdout. This activity uses no redirection or in-place editing.

### Counting too early

Incorrect idea:

```text
cut -d ',' -f 2 supplies.csv | wc -l
```

answers how many distinct categories exist.

Correction: that form counts all seven category records. Deduplicate before the count when the question asks for distinct values.

### Adding an unnecessary `cat`

Avoid:

```text
cat supplies.csv | cut -d ',' -f 2
```

Prefer:

```text
cut -d ',' -f 2 supplies.csv
```

`cut` can read the named file itself. Each stage should add a necessary operation.

### Using a more powerful tool without understanding

`awk`, scripting languages, and spreadsheet applications can solve related problems, but they would hide the lesson's small, composable transformations. The goal is not the shortest clever command. The goal is a pipeline whose stages the student can predict and explain.

## Deterministic completion checks

The activity verifies four exact stdout results:

1. seven complete records in alphabetical order;
2. three category names, each appearing once;
3. four uppercase zone names, each appearing once; and
4. a final line count of `3`.

Every task also verifies that:

- `supplies.csv` remains exactly unchanged; and
- the current directory remains `workspace/text-pipeline-lab`.

The current Primer verifier checks latest stdout and filesystem state but cannot fully prove pipeline structure. Exact instructions, ordered tasks, graduated hints, fixture preservation, and tutor questioning therefore remain important parts of assessment.

## Mastery questions

After deterministic checks pass, ask the student to answer in complete sentences:

1. Why must `sort` come before `uniq` in the category pipeline?
2. What does `cut -d ',' -f 3` mean, part by part?
3. In the zone pipeline, what text does `tr` receive, and what does it emit?
4. Why does the final `wc -l` print `3` even though the source has seven lines?
5. Why does `sort supplies.csv` leave the source file unchanged?
6. What assumption about this fixture makes basic comma-delimited `cut` safe here?

Mastery requires correct outputs and a clear left-to-right explanation. A memorized command without an explanation is incomplete.

## Parent or tutor notes

- Keep the fixture visible when first explaining fields, but ask the student to predict each intermediate stream rather than merely copy the final command.
- Require precise vocabulary: **record**, **field**, **delimiter**, **stdin**, **stdout**, **stage**, **adjacent**, and **distinct**.
- If the student treats `sort | uniq` as one mysterious operation, pause and demonstrate the adjacent-versus-separated duplicate distinction using the brief examples above.
- Reveal hints in order. Level 1 names the reasoning, level 2 names the command structure, and level 3 gives the exact command.
- Do not reward an opaque substitute merely because it prints matching output. The lesson assesses composition and explanation of the five named tools.
- If `supplies.csv` changes, reset the activity instead of repairing it through an unplanned command.

## Completion brief

The lesson is complete when the student:

- passes all four deterministic task checks in order;
- leaves `supplies.csv` byte-for-byte unchanged;
- remains in `workspace/text-pipeline-lab`;
- correctly explains each command's one purpose;
- explains why `uniq` needs adjacent equal lines and why `sort` provides them here;
- distinguishes selecting, transforming, ordering, deduplicating, and counting; and
- answers the mastery questions without relying on opaque tricks or unintroduced tools.
