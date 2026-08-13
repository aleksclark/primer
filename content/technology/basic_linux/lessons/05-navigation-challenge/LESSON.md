# Lesson 05: Navigation mastery challenge

## Objectives

By the end of this lesson, the student can:

- orient in an unfamiliar workspace before moving;
- discover a target by listing and inspecting supplied clues rather than guessing paths;
- distinguish ordinary names from hidden names;
- plan and follow a multi-component relative route;
- use the current working directory to interpret `.` and `..`; and
- report an reached location as a precise absolute path.

## Challenge brief

A field station has inherited an unfamiliar directory tree. Somewhere below the starting directory is an active relay bay. Each zone has an `INDEX.txt` card, but only one card identifies the active relay. The relay bay contains a hidden route card. That card gives a relative route to the correct archive.

Complete the mission in this order:

1. establish the exact starting location;
2. inspect the tree methodically and enter the active relay bay;
3. reveal the relay bay's hidden route card;
4. read and interpret the route; and
5. follow it to the archive and report the final location precisely.

The task text intentionally does not supply solution commands or the target path. Use the command knowledge from Lessons 01–04. If needed, request hints one level at a time; only the final hint for a task gives an exact command.

## Key ideas

**Orient, observe, plan, move, verify** is a reliable navigation cycle.

- **Orient:** establish the shell's current working directory. A relative path has no useful meaning until its starting directory is known.
- **Observe:** list the names at the current level. Inspect short clue files before choosing a branch.
- **Plan:** resolve every component of the intended path. Say where it should end before moving.
- **Move:** change the current working directory once the evidence supports the route.
- **Verify:** check the resulting location or inspect the expected names there.

Methodical discovery is different from trial-and-error wandering. Entering every directory until something looks right loses track of the cwd and does not show that the path was understood. In this challenge, the zone index cards let the student choose a branch while remaining at the starting location.

A name beginning with `.` is omitted from an ordinary listing. An all-names listing can reveal it. Hidden is only a naming convention; it does not make the item secret, inaccessible, or a different filesystem type.

The hidden card's route contains `..`. Resolve it from the relay bay, one component at a time. Each `..` selects the current directory's parent; it does not mean “undo my last command.” A route can move upward and then downward into a neighboring branch without returning all the way to the starting directory.

An absolute path begins with `/` and precisely reports a location from the filesystem root. The final report should describe where the shell actually arrived, not a path copied from a clue or inferred from the prompt.

## Command boundaries

This is a mastery challenge, not a new-command lesson. The available strategy is limited to the previously learned navigation and inspection tools:

- establish or report the cwd;
- list ordinary or all names;
- inspect a short known-text clue; and
- change the cwd with a relative path.

The brief names the purposes rather than giving a sequence to paste. Search commands, recursive traversal, glob-based shortcuts, pipelines, and filesystem-changing commands are outside the challenge. Discovery should come from reading one level and one clue at a time.

## Vocabulary

- **unfamiliar tree** — a directory structure whose useful route is not known at the start.
- **orientation** — establishing the cwd before interpreting relative paths.
- **systematic discovery** — choosing each inspection or move from evidence already observed.
- **route** — a path whose components lead from a known starting directory to a destination.
- **hidden name** — a name beginning with `.`, omitted by an ordinary listing.
- **relative path** — a path resolved from the current working directory.
- **absolute path** — a path beginning with `/` and resolved from the filesystem root.
- **verification** — checking that an observed result matches a prediction.

## Rules and safety

- Stay inside the supplied `workspace/field-station` fixture.
- Do not create, edit, copy, move, rename, or remove anything.
- Do not inspect the sandbox's real filesystem root.
- Use only the navigation and read-only inspection ideas already studied.
- Before changing directory, state the predicted destination.
- Directory fixtures use writable/traversable mode `0755` so the complete tree can be materialized safely. Read-only modes are used only for files, never directories.
- A reset restores the same deterministic tree and returns the shell to the starting directory.

## Challenge sequence

### 1. Establish the start

Report the cwd exactly. Identify which path components belong to the sandbox mount and which final components name this activity's fixture. Do not move yet.

### 2. Locate the active relay

Survey the names below the starting directory. Each zone contains an index card. Read those short cards from the starting location and use their evidence to identify the active zone. Inspect that zone's structure, plan a single relative path to its relay bay, and enter it.

### 3. Reveal the route card

List all names in the relay bay, including dot-prefixed names. Distinguish the hidden route card from the ordinary status file.

### 4. Interpret the route

Read the hidden card. Before moving, resolve its relative route component by component from the relay bay. Explain which parents it selects and which neighboring branch it then enters.

### 5. Reach and report the archive

Follow the route on the card. Confirm that the destination contains the expected sealed manifest, then report the destination's absolute path exactly. The report, cwd, and fixture state provide deterministic evidence of success.

## Mastery evidence

The terminal activity has five ordered tasks. It deterministically checks that the student:

1. reports the exact starting cwd;
2. reaches the active relay bay after independent discovery;
3. uses an all-names listing there and reveals `.route-card`;
4. reads the exact route card without changing it; and
5. reaches the designated archive and reports its exact absolute path.

The movement checks verify cwd state, while command and output checks verify the requested observation or report. Fixed clue contents make the challenge repeatable. Final checks also confirm that the hidden route card and sealed manifest remain unchanged.

The current contract observes only the most recent command, so exploratory listings and clue reads during the relay search are intentionally unscored. A parent or tutor should watch for a methodical process: orient, list, inspect both index cards from the known start, predict one route, move once, and verify. Success by random wandering should be reset and repeated with an explanation.

## Parent or tutor review

After completion, ask the student:

1. “What evidence told you which zone contained the active relay?”
2. “Why did an ordinary listing not show the route card?”
3. “From which cwd was the route on the card resolved?”
4. “What did each `..` component do?”
5. “How is the final absolute report different from the relative route you followed?”

A mastery response cites the index card, the route card's leading dot, the relay bay as the route's starting cwd, one parent transition per `..`, and `/` as the start of the final absolute path.

## Misconceptions

- **“The challenge expects me to guess directory names.”** Names and index cards provide enough evidence to discover the route.
- **“Faster means entering every branch until one works.”** Mastery means selecting a route from observations and predicting its result.
- **“A relative path starts at the activity root.”** It starts at the shell's cwd at the moment the path is resolved.
- **“`..` returns to my previous location.”** It selects the current directory's parent.
- **“A hidden file is absent or protected.”** Its leading dot only hides it from an ordinary listing.
- **“Listing a directory enters it.”** Inspection does not change the cwd.
- **“Reading the route changes the file.”** The supplied short-text reader prints contents without editing the fixture.
- **“The final report can be relative.”** The challenge asks for a precise absolute cwd report beginning at `/`.
- **“I should use a search command.”** Name-search tools are introduced later; this tree is designed for level-by-level discovery.

## Extension

Reset the activity and repeat the relay search while narrating every decision. Minimize directory changes: remain at the start while comparing the zone clues, then make one planned move into the relay bay. Do not memorize the solution command; explain why every path component is justified.

For an unscored path-literacy extension, stand in the final archive and verbally derive a relative route back to the active relay. Resolve the proposed route component by component, but do not run it. Then compare that route with the relay's absolute path and explain which form depends on the current cwd.
