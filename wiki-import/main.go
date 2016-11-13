package main

import (
	"container/list"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"os"
	"path"
	"regexp"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"

	"github.com/pkg/errors"

	"time"

	ui "github.com/gizak/termui"
)

type Site struct {
	Path         string
	Fetched      bool
	Size         int64
	LastModified time.Time
	HasRandom    bool

	Notes string

	fetchedThisRun int
}

type Data struct {
	RootUrl     string
	Sites       map[string]*Site
	SavePath    string
	RandomSites map[string]bool

	sitesAddedThisRound int
	siteToken           chan bool
}

type worker struct {
	num  int
	quit chan bool
	log  *log.Logger
	out  *listWriter
}
type listWriter struct {
	*ui.List
	buffer *list.List
}

const (
	NUM_WORKERS = 2
)

var includeRegex = regexp.MustCompile(`<<Include\(([^>]+)\)>>`)

var data Data

var workers []worker
var queue = make(chan *Site, 1024)
var queueSize sync.WaitGroup

var endChan = make(chan bool)
var end = false

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

	for i := 0; i < NUM_WORKERS; i++ {
		quit := make(chan bool)
		workers = append(workers, worker{num: i, quit: quit})
	}

	err := ui.Init()
	if err != nil {
		log.Fatalln(err)
	}
	defer ui.Close()
	centralLog := ui.NewList()
	centralLog.ItemFgColor = ui.ColorBlack
	centralLog.Overflow = "hidden"
	centralLog.Height = ui.TermHeight() / 2
	log.SetOutput(&listWriter{centralLog, list.New()})
	log.SetFlags(log.Ltime)

	workerRow := ui.NewRow()
	for i := range workers {
		el := ui.NewList()
		el.ItemFgColor = ui.ColorBlack
		el.Overflow = "hidden"
		el.Height = ui.TermHeight() / 2
		workerRow.Cols = append(workerRow.Cols, ui.NewCol(12/NUM_WORKERS, 0, el))
		workers[i].out = &listWriter{el, list.New()}
	}
	ui.Body.AddRows(
		ui.NewRow(
			ui.NewCol(12, 0, centralLog)),
		workerRow)
	ui.Body.Align()

	stop := func(e ui.Event) {
		log.Println("Got", e, "- exiting.")
		endChan <- true
		ui.StopLoop()
	}
	resize := func(ui.Event) {
		for i := range workers {
			workers[i].out.Height = ui.TermHeight() / 2
		}
		centralLog.Height = ui.TermHeight() / 2
		ui.Body.Width = ui.TermWidth()
		ui.Body.Align()
		ui.Render(ui.Body)
	}
	ui.Handle("/sys/kbd/q", stop)
	ui.Handle("/sys/kbd/C-c", stop)
	ui.Handle("/sys/kbd/<escape>", stop)
	ui.Handle("/sys/wnd/resize", resize)
	go func() {
		for {
			ui.Render(ui.Body)
			time.Sleep(50 * time.Millisecond)
		}
	}()

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
		data.RandomSites = map[string]bool{}
	}
	go func() {
		for _ = range endChan {
			end = true
			for _, w := range workers {
				w.quit <- true
			}
			save(data.SavePath)
		}
	}()

	data.siteToken = make(chan bool, 1)
	data.siteToken <- true

	for i := range workers {
		go workers[i].run()
	}

	go work()

	ui.Loop()
}

func work() {
	log.Println("Adding initial sites.")
	addSite("/", true)
	for _, site := range data.Sites {
		addSite(site.Path, true)
	}
	log.Println("running initial batch.")
	queueSize.Wait()

	for {
		if end {
			log.Println("ending -- someone asked to.")
			return
		}
		log.Println("next round. Using", len(data.RandomSites), "sites with random.")
		data.sitesAddedThisRound = 0
		for i := 0; i <= 30; i++ {
			for s := range data.RandomSites {
				addSite(s, true)
			}
			queueSize.Wait()
		}
		log.Println("new sites found in round:", data.sitesAddedThisRound)
		if data.sitesAddedThisRound == 0 {
			log.Println("Exiting.")
			break
		}
	}
	endChan <- true
}

func addSite(path string, force bool) {
	<-data.siteToken
	defer func() { data.siteToken <- true }()
	_, exists := data.Sites[path]
	if exists && !force {
		return
	}
	if !exists {
		log.Println("    new link:", path)
		data.sitesAddedThisRound++

		site := &Site{Path: path}
		data.Sites[path] = site
	}
	select {
	case queue <- data.Sites[path]:
		queueSize.Add(1)
	default:
		log.Println("dropping", path, "from queue - full.")
	}
}

func addRandomSite(path string) {
	<-data.siteToken
	defer func() { data.siteToken <- true }()
	if _, exists := data.RandomSites[path]; exists {
		return
	}
	log.Println("new randomSite", path)
	data.RandomSites[path] = true
}

func (w *listWriter) Write(b []byte) (int, error) {
	str := string(b)
	w.buffer.PushBack(str)
	for w.buffer.Len() > w.Height-2 {
		w.buffer.Remove(w.buffer.Front())
	}

	w.Items = make([]string, 0, w.buffer.Len())
	for e := w.buffer.Front(); e != nil; e = e.Next() {
		w.Items = append(w.Items, e.Value.(string))
	}
	ui.Render()
	return len(b), nil
}

func (w *worker) run() {
	w.log = log.New(w.out, "", 0)
	w.log.Println("Worker", w.num, "started")
	for {
		select {
		case <-w.quit:
			w.log.Println("Worker", w.num, "ended.")
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

		addSite(absolutePath, false)
	})
	s.fetchedThisRun++

	// get raw
	fileSavePath := data.SavePath + ".d" + s.Path + ".md"
	w.log.Println("downloading", siteUrl)

	req, err := http.NewRequest("GET", siteUrl+"?action=raw", nil)
	if err != nil {
		w.log.Println("  error:", err)
		return
	}
	if !s.LastModified.IsZero() {
		req.Header.Add("If-Modified-Since", s.LastModified.Format(time.RFC3339))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		w.log.Println("  error:", err)
		return
	}
	//w.log.Println("  headers:", resp.Header)
	if resp.StatusCode == 404 {
		return
	}
	if resp.StatusCode != 200 {
		w.log.Println("status:", resp.StatusCode, resp.Status)
		time.Sleep(10 * time.Second)
		return
	}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		w.log.Println("  error reading response:", err)
		return
	}
	s.Size = int64(len(b))
	err = os.MkdirAll(path.Dir(fileSavePath), 0755)
	if err != nil {
		w.log.Println("  error creating dirs:", path.Dir(fileSavePath), err)
		return
	}
	err = ioutil.WriteFile(fileSavePath, b, 0644)
	if err != nil {
		w.log.Println("  error writing file:", fileSavePath, err)
		return
	}
	w.log.Println("  wrote", s.Size, "bytes to", fileSavePath)

	bStr := string(b)
	if strings.Contains(bStr, "<<RandomPage") {
		s.HasRandom = true
		addRandomSite(s.Path)
	}
	matches := includeRegex.FindAllStringSubmatch(bStr, -1)
	for _, match := range matches {
		addSite("/"+match[1], false)
	}

	dateS := ""
	if resp.Header.Get("Last-Modified") != "" {
		dateS = resp.Header.Get("Last-Modified")
	} else if resp.Header.Get("Date") != "" {
		dateS = resp.Header.Get("Date")
	}
	if dateS != "" {
		s.LastModified, err = time.Parse(time.RFC1123, dateS)
		if err != nil {
			w.log.Println("error parsing last-modified:", err)
			s.LastModified = time.Now()
		}
	} else {
		s.LastModified = time.Now()
	}
}
