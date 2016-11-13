package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"os"
	"os/signal"
	"path"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"

	"github.com/pkg/errors"

	"time"
)

type Site struct {
	Path         string
	Fetched      int
	Size         int64
	LastModified time.Time
}

type Data struct {
	RootUrl  string
	Sites    map[string]*Site
	SavePath string
}

type worker struct {
	num  int
	quit chan bool
	log  *log.Logger
}

const (
	NUM_WORKERS = 3
)

var data Data

var workers []worker
var queue = make(chan *Site, 1024)
var sitesMutex sync.RWMutex
var queueSize sync.WaitGroup

func main() {
	defer func() {
		if got := recover(); got != nil {
			if err, ok := got.(error); ok {
				log.Printf("%+v", errors.WithStack(err))
			} else {
				log.Printf("%+v", errors.WithStack(errors.New(fmt.Sprint(got))))
			}
		}
	}()

	var url, savePath string
	var debug bool
	flag.StringVar(&url, "url", "", "Wiki Base URL")
	flag.StringVar(&savePath, "save", "wiki.json", "specify a savefile to work with")
	flag.BoolVar(&debug, "debug", false, "enable debug")
	flag.Parse()

	if debug {
		go func() {
			log.Fatalln(http.ListenAndServe("localhost:6060", nil))
		}()
	}

	loaded := load(savePath)
	if url == "" && !loaded {
		log.Println("No URL specified and no savefile found.")
		flag.Usage()
		return
	} else if url != "" {
		data.RootUrl = url
	}
	if strings.HasSuffix(data.RootUrl, "/") {
		data.RootUrl = strings.TrimSuffix(data.RootUrl, "/")
	}
	data.SavePath = savePath
	if data.Sites == nil {
		data.Sites = map[string]*Site{}
	}

	signalChan := make(chan os.Signal)
	signal.Notify(signalChan, os.Kill, os.Interrupt)
	go func() {
		for e := range signalChan {
			log.Println("Got", e, "- exiting.")
			queueSize.Add(100) // make sure nobody else does the exit
			end(1)
		}
	}()

	for i := 0; i < NUM_WORKERS; i++ {
		quit := make(chan bool)
		workers = append(workers, worker{num: i, quit: quit})
		go workers[i].run()
	}

	sitesMutex.Lock()
	if _, ok := data.Sites["/"]; !ok {
		queue <- &Site{Path: "/"}
		queueSize.Add(1)
	}
	for _, site := range data.Sites {
		if site.Fetched > 20 {
			continue
		}
		queue <- site
		queueSize.Add(1)
	}
	sitesMutex.Unlock()

	queueSize.Wait()
	end(0)
}
func end(code int) {
	for _, w := range workers {
		w.quit <- true
	}
	save(data.SavePath)
	os.Exit(code)
}

func (w *worker) run() {
	w.log = log.New(os.Stdout, fmt.Sprint("worker ", w.num, " "), log.Ltime)
	w.log.Println("Worker", w.num, "running")
	for {
		select {
		case <-w.quit:
			w.log.Println("Worker", w.num, "stopping")
			return
		case e := <-queue:
			w.processSite(e)
			queueSize.Done()
		}
	}
}

func load(path string) bool {
	b, err := ioutil.ReadFile(path)
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		panic(errors.WithMessage(err, "loading savefile"))
	}
	if json.Unmarshal(b, &data) != nil {
		panic(errors.WithMessage(err, "parsing savefile"))
	}
	return true
}
func save(path string) bool {
	log.Println("saving metadata to", path)
	b, err := json.MarshalIndent(&data, "", "  ")
	if err != nil {
		log.Println("error creating savefile:", err)
		return false
	}
	err = ioutil.WriteFile(path, b, 0644)
	if err != nil {
		log.Println("error writing savefile:", err)
		return false
	}
	log.Println("done.")
	return true
}

func (w *worker) processSite(s *Site) {
	rootUrl, _ := url.Parse(data.RootUrl)

	siteUrl := data.RootUrl + s.Path
	w.log.Println("processing", siteUrl)

	d, err := goquery.NewDocument(siteUrl)
	if err != nil {
		w.log.Println("error fetching document", siteUrl, err)
		return
	}
	goodLinks := 0
	sitesAdded := 0
	// get HTML, find links
	d.Find("a,link").Each(func(i int, e *goquery.Selection) {
		link, ok := e.Attr("href")
		if !ok {
			return
		}
		//w.log.Println("  found link", link)
		relativeUrl, err := url.Parse(link)
		if err != nil {
			w.log.Println("    invalid url", link, err)
			return
		}
		absoluteUrl := rootUrl.ResolveReference(relativeUrl)
		//w.log.Println("    absolute link:", absoluteUrl)
		absolutePath := absoluteUrl.EscapedPath()
		//w.log.Println("    escaped path:", absolutePath)

		if !strings.HasPrefix(absoluteUrl.String(), rootUrl.String()+"/") {
			//w.log.Println("    not wiki, ignoring.")
			return
		}
		absolutePath = strings.TrimPrefix(absolutePath, "/wiki")
		absolutePath, err = url.QueryUnescape(absolutePath)
		if err != nil {
			w.log.Println("    invalid unescape", absolutePath, err)
			return
		}
		goodLinks++

		sitesMutex.RLock()
		if _, exists := data.Sites[absolutePath]; exists {
			sitesMutex.RUnlock()
			return
		}
		sitesMutex.RUnlock()
		w.log.Println("    adding:", absolutePath)
		sitesAdded++

		sitesMutex.Lock()
		site := &Site{Path: absolutePath}
		data.Sites[absolutePath] = site
		select {
		case queue <- site:
			queueSize.Add(1)
		default:
			w.log.Println("dropping", absolutePath, "from queue - full.")
		}
		sitesMutex.Unlock()
	})
	if (goodLinks > 3 || sitesAdded > 0) && s.Fetched < 20 {
		select {
		case queue <- s:
			queueSize.Add(1)
		default:
			w.log.Println("not re-adding", s.Path, "to queue - full.")
		}
	}

	// get raw
	fileSavePath := data.SavePath + ".d" + s.Path + ".md"
	w.log.Println("downloading", siteUrl)
	resp, err := http.Get(siteUrl + "?action=raw")
	if err != nil {
		w.log.Println("  error:", err)
		return
	}
	w.log.Println("  headers:", resp.Header)
	if resp.StatusCode != 200 {
		w.log.Println("status:", resp.StatusCode, resp.Status)
		return
	}

	err = os.MkdirAll(path.Dir(fileSavePath), 0755)
	if err != nil {
		w.log.Println("  error creating dirs:", path.Dir(fileSavePath), err)
		return
	}
	f, err := os.Create(fileSavePath)
	if err != nil {
		w.log.Println("  error creating file:", fileSavePath, err)
		return
	}
	size, err := io.Copy(f, resp.Body)
	if err != nil {
		w.log.Println("  error writing file:", fileSavePath, err)
		return
	}
	s.Size = size
	s.Fetched++

	lastmod := resp.Header.Get("Last-Modified")
	if lastmod != "" {
		s.LastModified, err = time.Parse(time.ANSIC, lastmod)
		if err != nil {
			w.log.Println("error parsing last-modified:", err)
			s.LastModified = time.Now()
		}
	} else {
		s.LastModified = time.Now()
	}
	w.log.Println("  wrote", size, "bytes to", fileSavePath)
}
