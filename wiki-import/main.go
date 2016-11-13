package main

import (
	"bytes"
	"container/list"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"mime"
	"net/http"
	_ "net/http/pprof"
	"net/url"
	"os"
	"os/signal"
	"path"
	"regexp"
	"strings"
	"sync"

	. "github.com/cfstras/wiki-api/types"

	"github.com/PuerkitoBio/goquery"

	"github.com/pkg/errors"

	"time"

	ui "github.com/gizak/termui"
)

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

var (
	workers   []worker
	queue     = make(chan *Site, 1024)
	queueSize sync.WaitGroup
	waitStep  = sync.NewCond(&sync.Mutex{})
)

var (
	endChan = make(chan bool)
	end     = false
)

var (
	sitesAddedThisRound int
	siteToken           chan bool
)

func main() {
	var err error
	defer func() {
		if got := recover(); got != nil {
			if err, ok := got.(error); ok {
				log.Printf("%+v", errors.WithStack(err))
			} else {
				log.Printf("%+v", errors.WithStack(errors.New(fmt.Sprint(got))))
			}
		}
	}()

	var urlArg, savePath string
	var debug bool
	flag.StringVar(&urlArg, "url", "", "Wiki Base URL")
	flag.StringVar(&savePath, "save", "wiki.json", "specify a savefile to work with")
	flag.BoolVar(&debug, "debug", false, "enable debug")
	flag.Parse()

	if debug {
		go func() {
			log.Fatalln(http.ListenAndServe("localhost:6060", nil))
		}()
	}

	// load / parse root URL
	loaded := load(savePath)
	if urlArg == "" && !loaded {
		log.Println("No URL specified and no savefile found.")
		flag.Usage()
		return
	} else if urlArg != "" {
		if strings.HasSuffix(urlArg, "/") {
			urlArg = strings.TrimSuffix(urlArg, "/")
		}
		data.RootUrl, err = url.Parse(urlArg)
		if err != nil {
			log.Fatalln("parsing url", urlArg, err)
		}
	}

	// make workers
	for i := 0; i < NUM_WORKERS; i++ {
		quit := make(chan bool)
		workers = append(workers, worker{num: i, quit: quit})
	}

	// init UI
	err = ui.Init()
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

	signalChan := make(chan os.Signal)
	signal.Notify(signalChan, os.Kill)
	go func() {
		for _ = range signalChan {
			ui.Close()
			os.Exit(1)
		}
	}()

	stop := func(e ui.Event) {
		log.Println("Got", e, "- stopping.")
		endChan <- true
	}
	stopAndExit := func(e ui.Event) {
		stop(e)
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
	ui.Handle("/sys/kbd/q", stopAndExit)
	ui.Handle("/sys/kbd/C-c", stopAndExit)
	ui.Handle("/sys/kbd/<escape>", stop)
	ui.Handle("/sys/wnd/resize", resize)
	ui.Handle("/sys/kbd/<space>", func(ui.Event) {
		waitStep.Signal()
	})

	data.SavePath = savePath
	if data.Sites == nil {
		data.Sites = map[string]*Site{}
		data.RandomSites = map[string]bool{}
	}
	go func() {
		for _ = range endChan {
			if !end {
				end = true
				for _, w := range workers {
					w.quit <- true
					close(w.quit)
				}
			}
			save(data.SavePath)
		}
	}()

	siteToken = make(chan bool, 1)
	siteToken <- true

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
		sitesAddedThisRound = 0
		for i := 0; i <= 30; i++ {
			for s := range data.RandomSites {
				addSite(s, true)
			}
			queueSize.Wait()
		}
		log.Println("new sites found in round:", sitesAddedThisRound)
		if sitesAddedThisRound == 0 {
			log.Println("Exiting.")
			break
		}
	}
	endChan <- true
}

func addSite(path string, force bool) {
	<-siteToken
	defer func() { siteToken <- true }()
	_, exists := data.Sites[path]
	if exists && !force {
		return
	}
	if !exists {
		log.Println("    new link:", path)
		sitesAddedThisRound++

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
	<-siteToken
	defer func() { siteToken <- true }()
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
	ui.Render(ui.Body)
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
	s.Notes = ""

	siteUrl := data.RootUrl.String() + s.Path
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
		debug := false
		if debug {
			w.log.Println("  found link", link)
		}
		relativeUrl, err := url.Parse(link)
		if err != nil {
			w.log.Println("    invalid url", link, err)
			return
		}
		absoluteUrl := data.RootUrl.ResolveReference(relativeUrl)
		if debug {
			w.log.Println("    absolute link:", absoluteUrl)
		}
		action := absoluteUrl.Query().Get("action")
		var absolutePath string
		if action == "AttachFile" {
			q := absoluteUrl.Query()
			q.Set("do", "get")
			// delete unwanted query params
			for key := range q {
				if key != "do" && key != "action" && key != "target" {
					delete(q, key)
				}
			}
			absoluteUrl.RawQuery = q.Encode()
			target := absoluteUrl.Query().Get("target")
			if target == "" {
				return
			}
			absolutePath = absoluteUrl.RequestURI()
		} else {
			absolutePath = absoluteUrl.EscapedPath()
			if debug {
				w.log.Println("    escaped path:", absolutePath)
			}
		}

		if !strings.HasPrefix(absoluteUrl.String(), data.RootUrl.String()+"/") {
			if debug {
				w.log.Println("    not wiki, ignoring.")
			}
			return
		}
		absolutePath = strings.TrimPrefix(absolutePath, data.RootUrl.EscapedPath())
		absolutePath, err = url.QueryUnescape(absolutePath)
		if err != nil {
			w.log.Println("    invalid unescape", absolutePath, err)
			return
		}

		if debug {
			w.log.Println("    resolved to", absolutePath)
			w.doWaitStep()
		}
		addSite(absolutePath, false)
	})

	// get raw
	w.log.Println("downloading", siteUrl)

	parsedSiteUrl, err := url.Parse(siteUrl)
	if err != nil {
		w.log.Println("  invalid url", siteUrl, err)
		return
	}
	action := parsedSiteUrl.Query().Get("action")
	if action == "AttachFile" {
		w.log.Println("  as file.")
		target := parsedSiteUrl.Query().Get("target")
		safeUrl := parsedSiteUrl.Scheme + "://" + parsedSiteUrl.Host + "/" +
			parsedSiteUrl.Path + "?" +
			strings.Replace(parsedSiteUrl.RawQuery, " ", "%20", -1)
		w.log.Println("  escaped url:", safeUrl)

		filename := strings.TrimPrefix(parsedSiteUrl.EscapedPath(), data.RootUrl.EscapedPath())
		filename += "/" + target
		w.download(s, false, safeUrl, filename)
	} else {
		w.download(s, true, siteUrl+"?action=raw", s.Path)
	}
}

func (w *worker) download(s *Site, parse bool, downloadUrl, filename string) {
	s.Filename = filename
	req, err := http.NewRequest("GET", downloadUrl, nil)
	if err != nil {
		w.log.Println("  error:", err)
		w.doWaitStep()
		return
	}
	if !s.LastModified.IsZero() {
		req.Header.Add("If-Modified-Since", s.LastModified.Format(time.RFC3339))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		w.log.Println("  error:", err)
		w.doWaitStep()
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		s.Notes += resp.Status + "\n"
		return
	}
	if resp.StatusCode != 200 {
		w.log.Println("req:", downloadUrl)
		w.log.Println("status:", resp.StatusCode, resp.Status)
		s.Notes += resp.Status + "\n"
		w.doWaitStep()
		return
	}
	dateS := ""
	if resp.Header.Get("Last-Modified") != "" {
		dateS = resp.Header.Get("Last-Modified")
	} else if resp.Header.Get("Date") != "" {
		dateS = resp.Header.Get("Date")
	}
	if dateS != "" {
		newDate, err := time.Parse(time.RFC1123, dateS)
		if err != nil {
			w.log.Println("error parsing last-modified:", err)
			newDate = time.Now()
		}
		w.log.Println("  lastmod:", newDate, "- we have", s.LastModified)
		if !newDate.After(s.LastModified) {
			w.log.Println("    skipping download")
			return
		}
		s.LastModified = newDate
	} else {
		s.LastModified = time.Now()
	}
	fileSavePath := data.SavePath + ".d" + filename
	if parse {
		ext, err := w.getExt(resp.Header)
		if err != nil {
			w.log.Println("  could not get extensions", err)
			s.Notes += err.Error() + "\n"
			return
		}
		fileSavePath += ext
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		w.log.Println("  error reading response:", err)
		w.doWaitStep()
		return
	}
	s.Size = int64(len(b))
	err = os.MkdirAll(path.Dir(fileSavePath), 0755)
	if err != nil {
		w.log.Println("  error creating dirs:", path.Dir(fileSavePath), err)
		w.doWaitStep()
		return
	}
	err = ioutil.WriteFile(fileSavePath, b, 0644)
	if err != nil {
		w.log.Println("  error writing file:", fileSavePath, err)
		w.doWaitStep()
		return
	}
	w.log.Println("  wrote", s.Size, "bytes to", fileSavePath)

	if parse {
		bStr := string(b)
		if strings.Contains(bStr, "<<RandomPage") {
			s.HasRandom = true
			addRandomSite(s.Path)
		}
		matches := includeRegex.FindAllStringSubmatch(bStr, -1)
		for _, match := range matches {
			addSite("/"+match[1], false)
		}
	}

	buf := &bytes.Buffer{}
	resp.Header.Write(buf)
	s.Notes += buf.String() + "\n"
}

var mtypes = map[string]string{
	"text/plain":      ".txt",
	"image/png":       ".png",
	"image/jpeg":      ".jpeg",
	"image/gif":       ".gif",
	"application/pdf": ".pdf",
}

func (w *worker) getExt(header http.Header) (string, error) {
	contentType := header.Get("Content-Type")
	mtype, _, err := mime.ParseMediaType(contentType)
	if err == nil {
		if e, ok := mtypes[mtype]; ok {
			return e, nil
		}
	} else {
		return "", err
	}
	w.log.Println("unknown mimetype", mtype)
	w.doWaitStep()
	return "", errors.New("unknown mimetype " + mtype)
}

func (w *worker) doWaitStep() {
	w.log.Println("      <space>")
	waitStep.L.Lock()
	waitStep.Wait()
	waitStep.L.Unlock()
}
