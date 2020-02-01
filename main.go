package main

import (
	"bufio"
	"errors"
	//"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/net/html"
)

const (
	baseName         string = "results"
	fileType         string = ".json"
	maxFileSize      int64  = 2000 * 1024
	maxNumberOfFiles int    = 10
	basePath         string = "/home/leron/workspace/src/github.com/dimitarvalkanov7/temp/"
)

var (
	numberOfCreatedFiles int = 0
	mutex                sync.Mutex
	quit                 = make(chan struct{})
	page                 string
)

type PageContent struct {
	URL   string
	Words map[string]int
}

func main() {
	// Ask for the source file
	fmt.Print("Name of source file: ")
	var sourceFileName string
	fmt.Scanln(&sourceFileName)

	// Ask for the number of goroutines
	fmt.Print("\nNumber of concurrent executions: ")
	var numberOfRoutinesAsString string
	fmt.Scanln(&numberOfRoutinesAsString)
	numberOfGoroutines, _ := strconv.Atoi(numberOfRoutinesAsString)

	// Ask for execution time
	fmt.Print("\nMaximum execution time in seconds: ")
	var executionTimeAsString string
	fmt.Scanln(&executionTimeAsString)
	executionTime, _ := strconv.Atoi(executionTimeAsString)

	worklist := make(chan []string, numberOfGoroutines)

	//var n int // number of pending sends to worklist

	// Start with the command-line arguments.
	//n++

	initialURLs := getInitialData(sourceFileName)

	go func() {
		worklist <- initialURLs
	}()

	seen := make(map[string]bool)
	// for ; n > 0; n-- {
	// 	list := <-worklist
	// 	for _, link := range list {
	// 		if !seen[link] {
	// 			seen[link] = true
	// 			n++
	// 			go func(link string) {
	// 				worklist <- crawl(link)
	// 			}(link)
	// 		}
	// 	}
	// }

	expire := time.After((time.Duration(executionTime) * time.Second))
	//var wg sync.WaitGroup
	for /*numberOfCreatedFiles < maxNumberOfFiles*/ {
		select {
		case list := <-worklist:
			for _, link := range list {
				if !seen[link] {
					seen[link] = true
					//wg.Add(1)
					go func(link string) {
						worklist <- crawl(link)
						//defer wg.Done()
					}(link)
				}
			}
		case <-expire:
			fmt.Println("Operation took too long")
			//wg.Wait()
			close(worklist)
			fmt.Println("Exit")
			return
		case <-time.After(2000 * time.Millisecond):
			//wg.Wait()
			close(worklist)
			fmt.Println("Done")
			return
		case <-quit:
			// fmt.Println("Closing..")
			//wg.Wait()
			// close(worklist)
			close(worklist)
			// fmt.Println("Closed!")
			return
		}
	}
}

func getInitialData(fileName string) []string {
	fullPath := filepath.Join(basePath, fileName)

	file, err := os.Open(fullPath)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	urls := make([]string, 0)
	for scanner.Scan() {
		urls = append(urls, scanner.Text())
	}

	return urls
}

func crawl(url string) []string {
	//fmt.Println(url)

	list, err := extract(url)
	//	<-tokens // release the token
	if err != nil {
		log.Print(err)
	}
	return list
}

func fetchContentOfUrl(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", errors.New("Request went wrong")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", errors.New("Request went wrong")
	}

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	bodyString := string(bodyBytes)

	return htmlStringToJson(url, bodyString), nil
}

func extract(url string) ([]string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("getting %s: %s", url, resp.Status)
	}

	// TODO manage the content here?
	res, reqErr := fetchContentOfUrl(url)
	if reqErr != nil {
		return nil, reqErr
	}
	saveContent(res)
	// TODO end

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %s", url, resp.Status)
	}

	var links []string
	visitNode := func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, a := range n.Attr {
				if a.Key != "href" {
					continue
				}

				link, err := resp.Request.URL.Parse(a.Val)
				if err != nil {
					continue // ignore bad URLs
				}

				links = append(links, link.String())
			}
		}
	}

	forEachNode(doc, visitNode, nil)
	return links, nil
}

func forEachNode(n *html.Node, pre, post func(n *html.Node)) {
	if pre != nil {
		pre(n)
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		forEachNode(c, pre, post)
	}

	if post != nil {
		post(n)
	}
}

func saveContent(data string) {
	if numberOfCreatedFiles >= maxNumberOfFiles {
		quit <- struct{}{}
		return
	}

	mutex.Lock()
	l := len(page) + len(data)
	if int64(l) < maxFileSize {
		if len(page) < 10 {
			page = "[" + data
		} else {
			page = page + "," + data
		}
	} else {
		if len(page) < 10 {
			page = "[" + data + "]"
		} else {
			page = page + "," + data + "]"
		}
		numberOfCreatedFiles++
		var version string
		if numberOfCreatedFiles < 10 {
			version = "0" + strconv.Itoa(numberOfCreatedFiles)
		} else {
			version = strconv.Itoa(numberOfCreatedFiles)
		}

		fileName := "results" + version + fileType
		fullPath := filepath.Join(basePath, fileName)

		f, err := os.Create(fullPath)
		if err != nil {
			return
		}
		//log.Println(page)
		if _, err = f.WriteString(page); err != nil {
			log.Fatal(err)
		}
		page = "["
		defer f.Close()
	}
	mutex.Unlock()
}

func isLetter(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return true
}

func htmlStringToJson(url, body string) string {
	domDocTest := html.NewTokenizer(strings.NewReader(body))
	previousStartTokenTest := domDocTest.Token()
	page := new(PageContent)
	page.URL = url
	page.Words = make(map[string]int)

loopDomTest:
	for {
		tt := domDocTest.Next()
		switch {
		case tt == html.ErrorToken:
			break loopDomTest
		case tt == html.StartTagToken:
			previousStartTokenTest = domDocTest.Token()
		case tt == html.TextToken:
			if previousStartTokenTest.Data == "script" {
				continue
			}
			TxtContent := strings.TrimSpace(html.UnescapeString(string(domDocTest.Text())))
			if len(TxtContent) > 0 {
				words := strings.Split(TxtContent, " ")
				for _, v := range words {
					w := strings.TrimSpace(v)
					if len(w) > 2 && isLetter(w) {
						w = strings.ToLower(w)
						page.Words[w]++
					}
				}
			}
		}
	}

	result, _ := json.Marshal(page)
	return string(result)
}
