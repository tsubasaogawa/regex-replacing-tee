package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

const (
	confFileName              = "config.toml"
	defaultMode               = 0o644
	initConfigDefaultSentinel = "\x00default-config"
)

type rule struct {
	Name string `toml:"name"`
	From string `toml:"from"`
	To   string `toml:"to"`
}

type compiledRule struct {
	rule
	regexp *regexp.Regexp
}

type config struct {
	Rules []rule
}

type statistics struct {
	lines        int
	inputBytes   int
	outputBytes  int
	replacements int
}

type options struct {
	configPath     string
	append         bool
	overwrite      bool
	force          bool
	dryRun         bool
	preview        bool
	stats          bool
	checkConfig    bool
	listRules      bool
	listPresets    bool
	noDefaultRules bool
	presets        stringList
	initConfig     string
	showVersion    bool
	showHelp       bool
}

type stringList []string

func (values *stringList) String() string {
	return strings.Join(*values, ",")
}

func (values *stringList) Set(value string) error {
	*values = append(*values, value)
	return nil
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	defaultConfig, err := defaultConfigPath()
	if err != nil {
		fmt.Fprintf(stderr, "rrtee: determine default config path: %v\n", err)
		return 1
	}

	var options options
	options.configPath = defaultConfig
	flags := newFlagSet(stderr, &options)
	if err := flags.Parse(normalizeInitConfigArgument(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if options.showHelp {
		flags.Usage()
		return 0
	}
	if options.showVersion {
		fmt.Fprintln(stdout, version)
		return 0
	}
	if options.listPresets {
		if flags.NArg() != 0 {
			return argumentError(stderr, "--list-presets does not accept an output path")
		}
		listAvailablePresets(stdout)
		return 0
	}
	if options.initConfig == initConfigDefaultSentinel {
		options.initConfig = options.configPath
	}
	if options.initConfig != "" {
		if flags.NArg() != 0 {
			return argumentError(stderr, "--init-config does not accept an output path")
		}
		if err := initializeConfig(options.initConfig, options.force); err != nil {
			fmt.Fprintf(stderr, "rrtee: initialize config: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "initialized config: %s\n", options.initConfig)
		return 0
	}
	if options.append && options.overwrite {
		return argumentError(stderr, "--append and --overwrite cannot be used together")
	}
	if options.append && options.force {
		return argumentError(stderr, "--force cannot be used with --append")
	}
	if options.force {
		options.overwrite = true
	}
	if options.preview {
		options.dryRun = true
	}

	rules, err := activeRules(options)
	if err != nil {
		fmt.Fprintf(stderr, "rrtee: %v\n", err)
		return 1
	}
	if options.checkConfig {
		if flags.NArg() != 0 {
			return argumentError(stderr, "--check-config does not accept an output path")
		}
		fmt.Fprintf(stdout, "configuration OK: %d rule(s)\n", len(rules))
		return 0
	}
	if options.listRules {
		if flags.NArg() != 0 {
			return argumentError(stderr, "--list-rules does not accept an output path")
		}
		for _, entry := range rules {
			fmt.Fprintf(stdout, "%s\t%s\t%s\n", entry.Name, entry.From, entry.To)
		}
		return 0
	}

	if flags.NArg() > 1 {
		return argumentError(stderr, "expected one output path")
	}
	if !options.dryRun && flags.NArg() != 1 {
		return argumentError(stderr, "expected one output path")
	}

	var output io.Writer
	var closeOutput func() error
	if !options.dryRun {
		output, closeOutput, err = openOutput(flags.Arg(0), options)
		if err != nil {
			fmt.Fprintf(stderr, "rrtee: open output: %v\n", err)
			return 1
		}
	}

	stats, captureErr := capture(stdin, stdout, output, rules, options.preview, stderr)
	if closeOutput != nil {
		if err := closeOutput(); err != nil && captureErr == nil {
			captureErr = fmt.Errorf("close output: %w", err)
		}
	}
	if captureErr != nil {
		fmt.Fprintf(stderr, "rrtee: %v\n", captureErr)
		return 1
	}
	if options.stats || options.dryRun || options.preview {
		fmt.Fprintf(stderr, "rrtee: lines=%d input_bytes=%d output_bytes=%d replacements=%d\n",
			stats.lines, stats.inputBytes, stats.outputBytes, stats.replacements)
	}
	if options.dryRun {
		fmt.Fprintln(stderr, "rrtee: dry run; no output file written")
	}
	return 0
}

func newFlagSet(stderr io.Writer, options *options) *flag.FlagSet {
	flags := flag.NewFlagSet("rrtee", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: rrtee [options] OUTPUT")
		fmt.Fprintln(stderr, "Read stdin, pass it through unchanged to stdout, and write transformed text to OUTPUT.")
		fmt.Fprintln(stderr, "ANSI escape sequences are stripped from OUTPUT by default.")
		fmt.Fprintln(stderr, "")
		fmt.Fprintln(stderr, "Options:")
		flags.PrintDefaults()
	}

	flags.StringVar(&options.configPath, "c", options.configPath, "configuration file path")
	flags.StringVar(&options.configPath, "config", options.configPath, "configuration file path")
	flags.StringVar(&options.configPath, "conf", options.configPath, "configuration file path")
	flags.BoolVar(&options.append, "a", false, "append to OUTPUT")
	flags.BoolVar(&options.append, "append", false, "append to OUTPUT")
	flags.BoolVar(&options.overwrite, "overwrite", false, "replace an existing OUTPUT (requires --force)")
	flags.BoolVar(&options.force, "f", false, "allow --overwrite to replace an existing OUTPUT")
	flags.BoolVar(&options.force, "force", false, "allow --overwrite to replace an existing OUTPUT")
	flags.BoolVar(&options.dryRun, "n", false, "process input without writing OUTPUT")
	flags.BoolVar(&options.dryRun, "dry-run", false, "process input without writing OUTPUT")
	flags.BoolVar(&options.preview, "preview", false, "write transformed input to stderr instead of OUTPUT")
	flags.BoolVar(&options.stats, "stats", false, "write processing statistics to stderr")
	flags.BoolVar(&options.checkConfig, "check-config", false, "validate the active rules and exit")
	flags.BoolVar(&options.checkConfig, "check", false, "validate the active rules and exit")
	flags.BoolVar(&options.listRules, "list-rules", false, "list the active rules and exit")
	flags.BoolVar(&options.listRules, "rules", false, "list the active rules and exit")
	flags.BoolVar(&options.listPresets, "list-presets", false, "list built-in presets and exit")
	flags.BoolVar(&options.listPresets, "presets", false, "list built-in presets and exit")
	flags.BoolVar(&options.listPresets, "list", false, "list built-in presets and exit")
	flags.BoolVar(&options.noDefaultRules, "no-default-rules", false, "disable the default ANSI stripping rules")
	flags.Var(&options.presets, "preset", "enable a built-in preset (repeatable)")
	flags.StringVar(&options.initConfig, "init-config", "", "create a configuration file (PATH defaults to --config)")
	flags.StringVar(&options.initConfig, "init", "", "create a configuration file (PATH defaults to --config)")
	flags.BoolVar(&options.showVersion, "v", false, "print version and exit")
	flags.BoolVar(&options.showVersion, "version", false, "print version and exit")
	flags.BoolVar(&options.showVersion, "V", false, "print version and exit")
	flags.BoolVar(&options.showHelp, "h", false, "print help and exit")
	flags.BoolVar(&options.showHelp, "help", false, "print help and exit")
	return flags
}

func normalizeInitConfigArgument(args []string) []string {
	normalized := make([]string, 0, len(args))
	for index, argument := range args {
		if (argument == "--init-config" || argument == "-init-config" || argument == "--init" || argument == "-init") &&
			(index == len(args)-1 || strings.HasPrefix(args[index+1], "-")) {
			normalized = append(normalized, argument+"="+initConfigDefaultSentinel)
			continue
		}
		normalized = append(normalized, argument)
	}
	return normalized
}

func argumentError(stderr io.Writer, message string) int {
	fmt.Fprintf(stderr, "rrtee: %s\n", message)
	return 2
}

func activeRules(options options) ([]compiledRule, error) {
	rules := make([]rule, 0)
	if !options.noDefaultRules {
		rules = append(rules, ansiPreset()...)
	}
	for _, name := range options.presets {
		switch name {
		case "ansi":
			if options.noDefaultRules {
				rules = append(rules, ansiPreset()...)
			}
		case "none":
			rules = nil
			options.noDefaultRules = true
		default:
			return nil, fmt.Errorf("unknown preset %q (run --list-presets)", name)
		}
	}
	fromConfig, err := loadConfig(options.configPath)
	if err != nil {
		return nil, err
	}
	rules = append(rules, fromConfig.Rules...)
	return compileRules(rules)
}

func ansiPreset() []rule {
	return []rule{
		{Name: "ansi-csi", From: `\x1b\[[0-?]*[ -/]*[@-~]`, To: ""},
		{Name: "ansi-osc", From: `\x1b\][^\x07\x1b]*(\x07|\x1b\\)`, To: ""},
	}
}

func listAvailablePresets(output io.Writer) {
	fmt.Fprintln(output, "ansi\tStrip ANSI CSI and OSC escape sequences (enabled by default)")
	fmt.Fprintln(output, "none\tDo not add rules (use with --no-default-rules)")
}

func compileRules(rules []rule) ([]compiledRule, error) {
	compiled := make([]compiledRule, 0, len(rules))
	seen := make(map[string]bool, len(rules))
	for index, entry := range rules {
		if entry.Name == "" {
			entry.Name = fmt.Sprintf("rule %d", index+1)
		}
		if seen[entry.Name] {
			return nil, fmt.Errorf("duplicate rule name %q", entry.Name)
		}
		seen[entry.Name] = true
		re, err := regexp.Compile(entry.From)
		if err != nil {
			return nil, fmt.Errorf("invalid regex in rule %q (%q): %w", entry.Name, entry.From, err)
		}
		compiled = append(compiled, compiledRule{rule: entry, regexp: re})
	}
	return compiled, nil
}

func capture(input io.Reader, passthrough io.Writer, output io.Writer, rules []compiledRule, preview bool, diagnostics io.Writer) (statistics, error) {
	reader := bufio.NewReader(input)
	var stats statistics
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			stats.lines++
			stats.inputBytes += len(line)
			if _, writeErr := io.WriteString(passthrough, line); writeErr != nil {
				return stats, fmt.Errorf("write stdout: %w", writeErr)
			}

			transformed := line
			for _, entry := range rules {
				stats.replacements += len(entry.regexp.FindAllStringIndex(transformed, -1))
				transformed = entry.regexp.ReplaceAllString(transformed, entry.To)
			}
			stats.outputBytes += len(transformed)
			switch {
			case preview:
				fmt.Fprintf(diagnostics, "rrtee: preview: %s", transformed)
				if !strings.HasSuffix(transformed, "\n") {
					fmt.Fprintln(diagnostics)
				}
			case output != nil:
				if _, writeErr := io.WriteString(output, transformed); writeErr != nil {
					return stats, fmt.Errorf("write output: %w", writeErr)
				}
			}
		}
		if errors.Is(err, io.EOF) {
			return stats, nil
		}
		if err != nil {
			return stats, fmt.Errorf("read stdin: %w", err)
		}
	}
}

func openOutput(path string, options options) (io.Writer, func() error, error) {
	if path == "-" {
		return nil, nil, fmt.Errorf("output path %q is not supported; stdout is reserved for passthrough", path)
	}
	info, err := os.Stat(path)
	exists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, nil, err
	}
	if exists && info.IsDir() {
		return nil, nil, fmt.Errorf("%q is a directory", path)
	}

	var flags int
	switch {
	case options.append:
		flags = os.O_WRONLY | os.O_CREATE | os.O_APPEND
	case exists && options.overwrite && options.force:
		flags = os.O_WRONLY | os.O_TRUNC
	case exists && options.overwrite:
		return nil, nil, fmt.Errorf("%q already exists; add --force to overwrite it", path)
	case exists:
		return nil, nil, fmt.Errorf("%q already exists; use --append or --overwrite --force", path)
	default:
		flags = os.O_WRONLY | os.O_CREATE | os.O_EXCL
	}
	file, err := os.OpenFile(path, flags, defaultMode)
	if err != nil {
		return nil, nil, err
	}
	return file, file.Close, nil
}

func loadConfig(path string) (config, error) {
	if path == "" {
		return config{}, nil
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return config{}, nil
	} else if err != nil {
		return config{}, err
	}

	var raw struct {
		Rules toml.Primitive `toml:"rules"`
	}
	metadata, err := toml.DecodeFile(path, &raw)
	if err != nil {
		return config{}, fmt.Errorf("read config %q: %w", path, err)
	}
	if !metadata.IsDefined("rules") {
		return config{}, nil
	}

	var ordered []rule
	if err := metadata.PrimitiveDecode(raw.Rules, &ordered); err == nil {
		return config{Rules: ordered}, nil
	}

	var legacy map[string]rule
	if err := metadata.PrimitiveDecode(raw.Rules, &legacy); err != nil {
		return config{}, fmt.Errorf("decode rules in %q: rules must use [[rules]] (or legacy [rules.NAME]): %w", path, err)
	}
	names := make([]string, 0, len(legacy))
	for name := range legacy {
		names = append(names, name)
	}
	sort.Strings(names)
	ordered = make([]rule, 0, len(names))
	for _, name := range names {
		entry := legacy[name]
		entry.Name = name
		ordered = append(ordered, entry)
	}
	return config{Rules: ordered}, nil
}

func initializeConfig(path string, force bool) error {
	if path == "" {
		return errors.New("empty configuration path")
	}
	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if force {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}
	file, err := os.OpenFile(path, flags, defaultMode)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%q already exists; use --force to replace it", path)
		}
		return err
	}
	_, writeErr := io.WriteString(file, defaultConfigTemplate)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

const defaultConfigTemplate = `# rrtee rules are applied in the order written.
# ANSI escape sequences are stripped by the built-in default rules.
#
# [[rules]]
# name = "example"
# from = "secret=[^[:space:]]+"
# to = "secret=[redacted]"
`

func defaultConfigPath() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	if strings.Contains(executable, "go-build") {
		workingDirectory, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return filepath.Join(workingDirectory, confFileName), nil
	}
	return filepath.Join(filepath.Dir(executable), confFileName), nil
}

var version = "v0.0.0"
