# Lesson 01: Meet the Command Line

This lesson is designed for about one hour of focused work, but progress is based on mastery rather than time. Repeat guided practice or use hints as needed before completing the evidence tasks.

## Objectives

By the end of this lesson, the learner can:

- distinguish a terminal from a shell;
- identify the command and arguments in a command line;
- recognize the prompt as the shell's signal that it is ready for input;
- use `pwd` to report the current working directory;
- use `echo` to send chosen text to standard output;
- explain conceptually what `whoami` reports, without running it in this activity;
- compare input typed after a prompt with output printed by a command.

## Key ideas

### Terminal and shell are different

A **terminal** is the text-based interface where input and output appear. It gives you a place to type and a place for the computer to display text.

A **shell** is the program working inside the terminal. It reads a command line, interprets it, runs the requested command, displays the result, and then offers another prompt. A useful picture is:

1. the terminal displays the conversation;
2. the shell manages the conversation;
3. a command asks for a specific action.

The terminal is not itself the command, and the shell is not the prompt. They work together.

### Read the prompt before typing

The **prompt** is the shell's ready signal. It may show a user name, machine name, directory, or a symbol such as `$`. Its exact appearance varies. Do not type the prompt characters shown in an example; type only the command after them.

When a command finishes, the prompt normally returns. Text between the command and the next prompt is output. Some successful commands produce no visible output.

### A command line has parts

In this command line:

```text
echo Ready to learn
```

`echo` is the **command**. `Ready`, `to`, and `learn` are **arguments**: extra pieces of information passed to the command. Spaces usually separate the command and its arguments. Capital letters and punctuation matter because the shell treats the text literally.

`pwd` is commonly run with no arguments in beginner work:

```text
pwd
```

It prints the path of the **current working directory**, meaning the directory the shell is operating in now.

### Identity is not location

`whoami` conceptually asks, “Which user identity is running this shell?” Its answer is a user name. `pwd` asks, “Which directory am I working in?” Its answer is a path. These are different questions.

This lesson treats `whoami` conceptually rather than asking you to run it. Knowing the question a tool answers is part of reading command-line work accurately. A reported user identity may also differ from a user name shown in a customized prompt.

## Vocabulary

- **terminal**: the text interface that displays input and output;
- **shell**: the program that reads and interprets command lines;
- **prompt**: the shell's signal that it is ready for input;
- **command**: the action or program name at the start of a command line;
- **argument**: information supplied after a command;
- **input**: text or data sent to a program;
- **output**: text or data produced by a program;
- **current working directory**: the directory in which the shell is currently operating;
- **user identity**: the account under which a process is running.

## Guided practice

Work slowly enough to predict each result before pressing Enter.

1. Find the prompt. Notice any path or symbol it displays, but do not copy the prompt itself.
2. Type `pwd` and press Enter. Find the printed path between your command and the next prompt. Explain aloud why that path is output rather than part of the prompt.
3. Type `echo Ready` and press Enter. Point to the command and then to its one argument. Compare the input line with the output line.
4. Type `echo command argument` and press Enter. Identify `echo` as the command and `command` and `argument` as two arguments.
5. Predict whether `pwd` should print a user name or a directory path. Run it again and check the prediction.
6. Without running it, state what question `whoami` would answer. Contrast that question with the one answered by `pwd`.

The activity tasks provide graduated hints. First inspect the prompt and reread the relevant key idea. Use a level 1 hint for a reminder, level 2 for the command structure, and level 3 only when you need an exact recovery example.

## Mastery evidence

Mastery requires completing all four activity tasks in order:

1. run `pwd` and produce a path ending in the lesson workspace;
2. use `echo` with one exact argument and produce matching output;
3. use `echo` with three separate arguments, showing that the first word is the command and later words are arguments;
4. choose the accurate terminal-and-shell relationship from three contrasting statements and print it.

The checks are deterministic and inspect the latest command and its normalized output. Conceptual understanding should also be confirmed by asking the learner to explain:

- why the prompt is not part of the command;
- how terminal, shell, command, and argument relate;
- why `pwd` and `whoami` answer different questions.

If the learner can reproduce output but cannot explain these distinctions, give another example and retry rather than marking conceptual mastery.

## Misconceptions

- **“The terminal and shell are the same thing.”** The terminal provides the text interface; the shell interprets command lines within it.
- **“I should type the `$` from an example.”** The `$` usually represents a prompt. Type what follows it.
- **“Everything on screen is output.”** The prompt, typed input, command output, and error messages have different roles.
- **“Every word is a separate command.”** The first word names the command; following words are usually arguments to it.
- **“`pwd` tells me who I am.”** `pwd` reports a directory path. `whoami` reports a user identity when that tool is available.
- **“No output means failure.”** Some successful commands are silent. The returning prompt means the shell is ready again, not by itself that every command succeeded.

## Extension

Create three new `echo` command lines using only `echo` and your own harmless text. Before running each one, identify the command, count the arguments, and predict the exact output. Then run `pwd` once more and explain which parts of the screen are prompt, input, output, command, and argument. Finally, draw a four-box diagram showing the flow: learner → terminal → shell → command, with output returning to the terminal.
