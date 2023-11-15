// main
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

const (
	CONF_FILE = "config.toml"
)

type (
	// Configurations
	config struct {
		Rules map[string]rule
	}
	// Regexp rules
	rule struct {
		From string // Regexp rule
		To   string // Replace to
	}
)

var (
	version  string = "v0.0.0"
	confFile string
	v        bool
)

func init() {
	c := fmt.Sprintf("%s/%s", getAppDir(), CONF_FILE)

	flag.BoolVar(&v, "v", false, "version")
	flag.StringVar(&confFile, "c", c, "config file path")
}

func main() {
	flag.Parse()

	if v {
		fmt.Println(version)
		os.Exit(0)
	}

	if flag.NArg() != 1 {
		flag.PrintDefaults()
		os.Exit(0)
	}

	out := flag.Args()[0]
	f, err := os.OpenFile(out, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	capture(f, loadConf(confFile))
}

// Capturing stdin with replacing text
func capture(f *os.File, c *config) {
	regexs := map[string]*regexp.Regexp{}

	for k, v := range c.Rules {
		regexs[k] = regexp.MustCompile(v.From)
	}

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		t := scanner.Text()

		// To console
		println(t)
		// To file: replace for the number of regexs
		for k, v := range regexs {
			t = fmt.Sprint(v.ReplaceAllString(t, c.Rules[k].To))
		}
		fmt.Fprintln(f, t)
	}
	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
}

// Load configuration toml file
func loadConf(confFile string) *config {
	conf := config{}

	if exists(confFile) {
		if _, err := toml.DecodeFile(confFile, &conf); err != nil {
			log.Fatal(err)
		}
	}

	return &conf
}

// Returns application abs path. If the app was run by `go run`, it returns cwd
func getAppDir() string {
	d, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}

	if !isGoRun(d) {
		return filepath.Dir(d)
	}

	d, err = os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	if !exists(filepath.Join(d, "main.go")) {
		log.Fatal("Please `go run` in the script directory")
	}

	return d
}

func isGoRun(_path string) bool {
	return strings.Contains(_path, "go-build")
}

func exists(_path string) bool {
	_, err := os.Stat(_path)
	return err == nil
}
