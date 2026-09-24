# regex-replacing-tee (rrtee)

`rrtee` copies standard input in two forms: the original text remains available
on standard output, while a configured sequence of regular-expression
replacements is written to an output file. It is useful when terminal output
contains ANSI escape sequences that should not be saved.

## Quick start

Strip ANSI SGR colour and style sequences from a command's saved output while
leaving the coloured terminal output untouched:

```bash
ls -l --color=always /var | rrtee --preset ansi var-listing.txt
```

The command above writes the incoming bytes to standard output and the
colour-free result to `var-listing.txt`.

## Usage

```text
rrtee [--config PATH] [--preset NAME] [--append | --force] OUTPUT
rrtee --preview [--config PATH] [--preset NAME]
rrtee --check [--config PATH] [--preset NAME]
rrtee --list
rrtee --init [PATH]
```

`-c PATH` is the short form of `--config PATH`. `-v` prints the version.

### Normal output

In normal mode, `rrtee` reads lines from standard input, sends each original
line to standard output, and writes its transformed counterpart to `OUTPUT`.
Rules apply in the order in which they occur in the configuration.

For safety, a normal run does not replace an existing output file. Choose the
intended behavior explicitly:

```bash
# Add transformed input to an existing log.
make 2>&1 | rrtee --preset ansi --append build.log

# Replace an existing file.
make 2>&1 | rrtee --preset ansi --force build.log
```

Without `--append` or `--force`, choose a new output path (or remove the old
file) instead. `--append` and `--force` are mutually exclusive.

### Preview

`--preview` applies the selected rules but does not create, append to, or
replace a file. The transformed result is written to standard output, making
it suitable for checking a pipeline:

```bash
printf '\033[31mwarning\033[0m\n' | rrtee --preset ansi --preview
# warning
```

### Check configuration

Use `--check` to load and validate a configuration and preset selection
without consuming standard input or opening an output file:

```bash
rrtee --config config.toml --check
rrtee --preset ansi --check
```

It exits successfully only when every selected rule is valid.

### Presets

`ansi` is the built-in preset for removing ANSI SGR colour and style escape
sequences. It is equivalent to the rules in the sample `config.toml`:

```bash
git -c color.ui=always status --short | rrtee --preset ansi status.txt
```

List the available preset names with:

```bash
rrtee --list
```

### Create a configuration

Create a starter configuration with `--init`. With no path, it writes
`config.toml` in the current directory; pass a path to choose another file:

```bash
rrtee --init
rrtee --init ~/.config/rrtee/rules.toml
```

`--init` follows the same output-safety rule: it will not overwrite an
existing configuration unless the overwrite option is explicitly supplied.

## Configuration

Configuration is TOML. Each `[[rules]]` table defines one replacement:

```toml
[[rules]]
from = 'secret=[^[:space:]]+'
to = 'secret=[REDACTED]'

[[rules]]
from = 'warning'
to = 'WARNING'
```

Rules are ordered: the first replacement runs first, and its result becomes
the input to the next rule. Use TOML literal strings (`'...'`) when possible
so regular-expression backslashes are not interpreted by TOML. `from` is a Go
regular expression and `to` uses Go replacement-string syntax (for example,
`$1` for the first capture group).

Use a configuration for normal output, preview, or checking:

```bash
command | rrtee --config config.toml output.txt
command | rrtee --config config.toml --preview
rrtee --config config.toml --check
```

The supplied `config.toml` preserves the historic ANSI-cleaning behavior.

## Streams and diagnostics

| Mode | Standard input | Standard output | Files |
| --- | --- | --- | --- |
| Normal | Original input | Original, unchanged input | Transformed output file |
| `--preview` | Original input | Transformed input | None |
| `--check` | Not read | Validation result | None |
| `--list` | Not read | Available presets | None |
| `--init` | Not read | Initialization result | New configuration file |

Errors and usage diagnostics are written to standard error, so standard
output can remain part of a pipeline.
