package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/antonholmquist/jason"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/comprehend"
	"github.com/microcosm-cc/bluemonday"
)

type kv struct {
	Key   string
	Value int
}

var c *comprehend.Comprehend
var wordCount = make(map[string]int)
var linkCount = make(map[string]bool)

func sortMap(m map[string]int) []kv {
	var ss []kv
	for key, count := range wordCount {
		ss = append(ss, kv{key, count})
	}

	sort.Slice(ss, func(i, j int) bool {
		return ss[i].Value > ss[j].Value
	})

	return ss
}

func getKeyPhrases(s string) (*comprehend.DetectSyntaxOutput, error) {
	var dKPI comprehend.DetectSyntaxInput
	keyInput := dKPI.SetLanguageCode("en")

	keyInput = dKPI.SetText(s)
	err := keyInput.Validate()
	if err != nil {
		return nil, err
	}

	keysOut, err := c.DetectSyntax(keyInput)
	if err != nil {
		return nil, err
	}

	return keysOut, nil
}

func checkForWords(url string) {
	var err error
	defer func() {
		if err != nil {
			log.Printf("error in keyterms handler - error: %v", err)
			os.Exit(1)
		}
	}()

	resp, _ := http.Get(url)
	bytes, _ := ioutil.ReadAll(resp.Body)
	v, err := jason.NewObjectFromBytes(bytes)
	if err != nil {
		return
	}

	body, err := v.GetString("body")
	if err != nil {
		return
	}

	for _, paragraph := range strings.Split(body, "</p>") {
		bM := bluemonday.StrictPolicy().Sanitize(paragraph)

		if len(bM) <= 0 {
			continue
		}

		keysOut, err := getKeyPhrases(bM)
		if err != nil {
			log.Printf("error in keyterms handler - error: %v", err)
			os.Exit(1)
		}

		for _, kP := range keysOut.SyntaxTokens {
			if *kP.PartOfSpeech.Tag != "VERB" && *kP.PartOfSpeech.Tag != "NOUN" && *kP.PartOfSpeech.Tag != "PROPN" && *kP.PartOfSpeech.Tag != "ADV" {
				continue
			}

			key := strings.Join([]string{*kP.Text, " (", *kP.PartOfSpeech.Tag, ")"}, "")
			wordCount[key]++
		}
	}

	linkCount[url] = true
	println("parsed", url)
}

func main() {
	fp := flag.String("file", "", "a single file path to read links from.")
	flag.Parse()

	var err error
	defer func() {
		if err != nil {
			log.Printf("error in keyterms handler - error: %v", err)
			os.Exit(1)
		}
	}()

	println("start")
	if *fp == "" {
		err = errors.New("file path containing links is required")
		return
	}

	file, err := os.Open(*fp)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	links := make(map[string]bool)

	println("reading links")

	for scanner.Scan() {
		link := scanner.Text()
		links[link] = true
	}

	println("creating a new AWS session")
	session, err := session.NewSession(&aws.Config{Region: aws.String("us-east-1")})
	if err != nil {
		return
	}

	c = comprehend.New(session)

	for link := range links {
		checkForWords(link)
	}

	sWC := sortMap(wordCount)

	println("\n--------------------")
	println("Report Finished:")
	println("link checked: ", len(linkCount))
	println("word count: ", len(sWC), "\n")
	println("Top used words:")
	for i, kv := range sWC {
		fmt.Printf("%d\t%s\n", kv.Value, kv.Key)
		if i >= 25 {
			break
		}
	}
}
