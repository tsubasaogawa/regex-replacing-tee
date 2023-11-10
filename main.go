package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"regexp"
)

func main() {
	r := regexp.MustCompile(`ogawa`)
	f, err := os.OpenFile("/tmp/ogawa.log", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		t := scanner.Text()
		println(t)
		fmt.Fprintln(f, r.ReplaceAllString(t, ""))
	}
	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
}
