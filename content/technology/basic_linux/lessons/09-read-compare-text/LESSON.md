# Lesson 09: Read and compare text

## Objectives

By the end of this lesson, the student can:

- use `cat FILE` to print one complete short text file;
- use `head -n COUNT FILE` to select a file's first lines;
- use `tail -n COUNT FILE` to select a file's final lines;
- use `wc -l FILE` to count lines without printing and counting every record by eye;
- use `diff OLD NEW` to locate line-by-line differences and interpret ordinary diff markers;
- explain that `diff` status 1 means the files differ, not that comparison failed;
- choose the smallest useful read-only tool for a precise question; and
- inspect text while preserving source paths and contents exactly.

## Standards

- **Primary — `PRIMER.DL.6.FILES.2`:** Inspect file contents with safe read-only tools and confirm expected text. The student reads complete and selected text, obtains a line count, and compares two versions with deterministic output checks.
- **Reinforcement — `PRIMER.DL.6.FILES.1`:** Organize a small workspace according to an explicit structure. Here the student reinforces respect for established filenames, version order, and unchanged source material while inspecting an organized lab.

## Key ideas

A command should answer the question without producing unnecessary information. That principle is **the smallest useful tool**: first decide what evidence is needed, then choose the command whose normal output most directly supplies it.

Suppose a file has hundreds of lines. Printing every line with `cat` can technically reveal its first line, last line, and number of records, but it gives too much output and makes the student perform work a more focused command can do reliably. Choose by question:

- “What does this entire short note say?” → `cat`
- “What are the first three records?” → `head -n 3`
- “What are the final two records?” → `tail -n 2`
- “How many lines are there?” → `wc -l`
- “What changed between these versions?” → `diff`

These commands inspect data rather than intentionally editing it. Read-only does not mean careless: exact filenames, option spelling, operand order, and output interpretation still matter.

### Read a complete short file with `cat`

`cat FILE` writes the file's contents to standard output:

```console
$ cat notice.txt
Observatory closes at dusk.
Return the key to the red box.
```

This is a good fit because the notice is short and the question asks for all of it. It would be a poor fit for a very long log when only one end matters.

### Select the beginning with `head`

`head -n COUNT FILE` prints the requested number of lines from the beginning:

```console
$ head -n 3 journal.txt
06:00 gate opened
06:15 lens uncovered
06:30 sky clear
```

Here `3` is a count: print three lines. It does not mean “jump to line 3.”

### Select the ending with `tail`

`tail -n COUNT FILE` prints the requested number of lines from the end:

```console
$ tail -n 2 journal.txt
07:00 clouds approaching
07:15 session paused
```

`head` and `tail` are complementary. Select between them by which end the question names.

### Count lines with `wc -l`

`wc` means word count, but its options choose what to count. The `-l` option reports lines:

```console
$ wc -l specimens.txt
5 specimens.txt
```

The output includes the count and, because a filename operand was supplied, the filename. `wc -l` counts newline characters. In ordinary lesson files, each record ends with a newline, so the result is the number of records. Screen wrapping does not create additional file lines.

### Compare versions with `diff`

`diff FIRST SECOND` compares text line by line. Operand order gives the markers meaning:

- `<` introduces text from the first file;
- `>` introduces text from the second file;
- `---` separates an old block from its replacement;
- a header such as `2c2` says line 2 changed to line 2;
- a header such as `4a5` says that after old line 4, new line 5 was added.

```console
$ diff checklist-old.txt checklist-new.txt
2c2
< filter: blue
---
> filter: green
4a5
> tripod
```

This reports that `filter: blue` in the old file became `filter: green` in the new file and that `tripod` was added to the new version.

Unlike most commands used so far, `diff` uses three meaningful status classes:

- status 0: the files are identical;
- status 1: the files differ and the output describes those differences;
- status greater than 1: a problem prevented a normal comparison.

Therefore status 1 is expected in this activity. Do not edit the files merely to make `diff` return 0.

## Vocabulary

- **text file** — a file whose bytes represent readable characters arranged into lines.
- **standard output** — the ordinary output stream where these commands print results.
- **read-only inspection** — observing information without intentionally changing source contents or paths.
- **smallest useful tool** — the command whose scope directly answers the question without unnecessary output or manual work.
- **`cat`** — a command that writes complete file contents to standard output; useful here for one short file.
- **`head`** — a command that selects lines from the beginning of input.
- **`tail`** — a command that selects lines from the end of input.
- **`wc`** — a command that counts aspects of text; `wc -l` counts newline-terminated lines.
- **line** — text ending at a newline character; terminal wrapping does not add file lines.
- **`diff`** — a command that reports line-by-line differences between two files.
- **operand order** — the left-to-right order of paths supplied to a command; for `diff`, it determines first/old and second/new markers.
- **exit status** — a number summarizing a command result; for `diff`, 1 specifically means differences found.
- **fixture** — supplied, resettable practice data whose exact contents support deterministic checking.

## Command choice

| Question | Smallest useful form | Why |
|---|---|---|
| What does this complete short file say? | `cat FILE` | Prints all requested text directly |
| What are the first *N* lines? | `head -n N FILE` | Limits output to the requested beginning |
| What are the final *N* lines? | `tail -n N FILE` | Limits output to the requested ending |
| How many lines are in the file? | `wc -l FILE` | Computes the count instead of requiring visual counting |
| Which lines differ between two versions? | `diff OLD NEW` | Reports replacements, additions, and removals in version order |

The best choice follows the information need, not whichever command was learned first. Before typing, complete this sentence: “The question asks for ___, so I will use ___ because its output contains ___ and avoids ___.”

## Guided practice

The activity opens in `workspace/text-lab` with five read-only fixture files:

```text
text-lab/
├── checklist-new.txt
├── checklist-old.txt
├── journal.txt
├── notice.txt
└── specimens.txt
```

Complete five tasks in order:

1. **Read the whole notice.** The notice has only two lines and the question asks for both. Run `cat notice.txt` and read the entire result.
2. **Inspect the journal opening.** The question asks only for the first three entries. Run `head -n 3 journal.txt` and verify that output stops after `06:30 sky clear`.
3. **Inspect the journal ending.** The question asks only for the final two entries. Run `tail -n 2 journal.txt` and verify that output begins at `07:00 clouds approaching`.
4. **Count specimen records.** Run `wc -l specimens.txt`. Read `5` as the line count; do not print every specimen and count by eye.
5. **Compare checklist versions.** Run `diff checklist-old.txt checklist-new.txt`. Explain the changed filter and added tripod using the `<` and `>` markers. Accept status 1 as the expected differences-found result.

Do not redirect output, pipe commands together, create an answer file, or edit source files. The task is command selection and interpretation, not storing results.

## Mastery evidence

The terminal activity uses five ordered tasks and deterministic checks:

1. Exact `cat notice.txt` command and exact two-line output prove appropriate whole-file reading.
2. Exact `head -n 3 journal.txt` command and exact three-line output prove beginning selection with an explicit count.
3. Exact `tail -n 2 journal.txt` command and exact two-line output prove ending selection with an explicit count.
4. Exact `wc -l specimens.txt` command plus a whitespace-tolerant anchored output check prove that the student requested a line count and obtained five.
5. Exact `diff checklist-old.txt checklist-new.txt`, expected status 1, and exact normal diff output prove correct operand order and comparison result.

Independent `content_equals` checks establish that every source file remains exact. Working-directory checks establish that the student remains inside the bounded lab. Exact command checks prevent broad substitutes such as printing all records and counting visually from satisfying a focused task.

The contract can verify command form, latest output, exit status, cwd, and fixture contents. It cannot directly score the student's spoken explanation. A parent or tutor should require the student to state the question, selected tool, expected output scope, and interpretation before advancing.

## Parent or tutor review

Ask the student to answer without typing more commands:

1. Why is `cat` appropriate for `notice.txt` but not the smallest useful choice for the first three journal entries?
2. In `head -n 3 journal.txt`, what does `3` mean?
3. How would the result differ if `tail` replaced `head`?
4. Why is `wc -l` more reliable than counting a printed list by eye?
5. Does a visually wrapped terminal row necessarily count as another file line? Why not?
6. In the diff output, which marker identifies text from the old file? Which identifies the new file?
7. What does `2c2` communicate?
8. What does `4a5` communicate?
9. Why is status 1 successful evidence for this particular `diff` task?
10. What would a `diff` status greater than 1 suggest?
11. Which checks prove that inspection did not change the files?
12. Give one new question suited to each of the five commands.

Require precise use of beginning, end, line count, operand order, first file, second file, exit status, standard output, and smallest useful tool.

## Misconceptions

- **“`cat` is the universal reading command.”** It can print a file, but focused questions often call for narrower output from `head`, `tail`, `wc`, or `diff`.
- **“The number after `-n` is a line number.”** In these forms it is a count of lines to print from one end.
- **“`head` and `tail` summarize the file.”** They select literal lines from the beginning or end; they do not interpret or summarize.
- **“`wc -l` always counts human-visible paragraphs.”** It counts newline characters, not paragraphs or wrapped display rows.
- **“The filename in `wc -l` output is another result.”** It labels the operand whose count is shown.
- **“`diff` edits or merges the files.”** Plain `diff` only reports differences.
- **“Operand order does not matter to `diff`.”** Reversing operands reverses which text is marked `<` and `>` and changes the edit description.
- **“Any nonzero status means failure.”** For `diff`, status 1 means the comparison completed and found differences.
- **“No diff output means the command did nothing.”** With status 0, silence means the files are identical.
- **“Read-only work needs no verification.”** Exact paths, focused output, unchanged contents, and status interpretation remain part of careful work.

## Tutor strategy and constraints

Use the same graduated sequence for every task:

1. Ask the student to restate the exact question: complete text, beginning, end, count, or differences.
2. Ask which command naturally produces only that kind of evidence and what arguments constrain its scope.
3. Ask the student to predict the shape of output, including line count or diff markers.
4. Reveal the exact current command only at hint level 3.
5. After execution, require interpretation before moving on.

Keep coaching within `cat`, `head -n`, `tail -n`, `wc -l`, and plain two-file `diff`. Do not introduce pipes, redirection, `grep`, `less`, `sed`, `awk`, command substitution, loops, sorting, aliases, or command chains. Do not accept an overbroad command merely because the answer could be extracted manually from its output.

For the comparison task, preserve old-before-new operand order and explicitly teach `diff` status semantics. Never direct the student to edit fixtures to produce status 0. If a command or output differs unexpectedly, compare spelling, count, option, path, cwd, and operand order; reset rather than improvising file changes.

## Safety and craftsmanship

- Work only in `workspace/text-lab`.
- Treat all five supplied files as source evidence; do not alter, rename, copy, or remove them.
- Use one focused command per task, with no pipe, redirection, or command chaining.
- Read options carefully: lowercase `-n` for selected line counts and lowercase `-l` for line count.
- Supply only the requested relative file paths.
- Preserve `checklist-old.txt` before `checklist-new.txt` in the `diff` command.
- Interpret output rather than merely producing it.
- Distinguish `diff` status 1 from a comparison error.
- Prefer limited output that directly answers the question.
- Verify preservation as part of successful inspection.

## Extension

Use verbal or paper answers only; do not run commands in the assessed fixture. For each question, name the smallest useful tool and justify it:

1. A three-line emergency card must be read in full.
2. A 2,000-line sensor log needs its first five records checked.
3. The same log needs its final alert checked.
4. A roster needs only its number of lines reported.
5. Two policy drafts need their textual changes identified.
6. Two files need to be tested for equality, and `diff` prints nothing with status 0.
7. A long file needs every line displayed even though only its last two are relevant.

For item 7, reject the proposed broad display and choose `tail -n 2`. For every answer, identify what information is requested, what the chosen command emits, and what unnecessary work the other tools would create.
