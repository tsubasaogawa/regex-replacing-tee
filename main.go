package main

import (
	"bufio"
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
	config struct {
		Rules map[string]rule
	}
	rule struct {
		From string
		To   string
	}
)

func main() {
	c := loadConf()

	r := map[string]*regexp.Regexp{}
	for k, v := range c.Rules {
		r[k] = regexp.MustCompile(v.From)
	}

	f, err := os.OpenFile("/tmp/ogawa.log", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		t := scanner.Text()
		// To console
		println(t)
		// To file
		for k, v := range r {
			t = fmt.Sprint(v.ReplaceAllString(t, c.Rules[k].To))
		}
		fmt.Fprintln(f, t)
	}
	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
}

func loadConf() *config {
	c := fmt.Sprintf("%s/%s", getAppDir(), CONF_FILE)
	var conf config
	if exists(c) {
		if _, err := toml.DecodeFile(c, &conf); err != nil {
			log.Fatal(err)
		}
	}

	return &conf
}

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
