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
	"strconv"
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
	endChan  = make(chan bool)
	end      = false
	uiIsInit = false
)

var (
	sitesAddedThisRound int
	siteToken           chan bool
)

func panicrecover() {
	if uiIsInit {
		ui.Close()
		uiIsInit = false
	}
	if got := recover(); got != nil {
		if err, ok := got.(error); ok {
			fmt.Printf("%+v\n", errors.WithStack(err))
		} else {
			fmt.Printf("%+v\n", errors.WithStack(errors.New(fmt.Sprint(got))))
		}
		os.Exit(1)
	}
}

func main() {
	var err error
	defer panicrecover()

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
	fmt.Println("args parsed")

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
	fmt.Println("loaded")

	// make workers
	for i := 0; i < NUM_WORKERS; i++ {
		quit := make(chan bool)
		workers = append(workers, worker{num: i, quit: quit})
	}
	fmt.Println("workers inited")

	// init UI
	err = ui.Init()
	if err != nil {
		log.Fatalln(err)
	}
	uiIsInit = true
	defer func() {
		if uiIsInit {
			ui.Close()
			uiIsInit = false
		}
	}()

	fmt.Println("ui inited")
	centralLog := ui.NewList()
	centralLog.ItemFgColor = ui.ColorBlack
	centralLog.Overflow = "hidden"
	centralLog.Height = ui.TermHeight() / 2
	log.SetOutput(&listWriter{centralLog, list.New()})
	log.SetFlags(log.Ltime)
	fmt.Println("log inited")

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
	go func(ch <-chan time.Time) {
		for _ = range ch {
			log.Println("queue has", len(queue), "elements, saving")
			save(data.SavePath)
		}
	}(time.Tick(10 * time.Second))
	/*go func(ch <-chan time.Time) {
		for _ = range ch {
			ui.Render(ui.Body)
		}
	}(time.Tick(100 * time.Millisecond))*/

	data.SavePath = savePath
	if data.Sites == nil {
		data.Sites = map[SiteKey]*Site{}
		data.RandomSites = map[SiteKey]bool{}
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

func (w *worker) doWaitStep() {
	w.log.Println("      <space to continue>")
	waitStep.L.Lock()
	waitStep.Wait()
	waitStep.L.Unlock()
}

func work() {
	log.Println("Adding initial sites.")
	addSite(SiteKey{"/", false}, true)
	for _, site := range data.Sites {
		addSite(site.Key, true)
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

func addSite(siteKey SiteKey, force bool) {
	site := &Site{Key: siteKey, Revisions: map[int]*Revision{}}
	if _, err := url.Parse(site.Key.SiteUrl(data)); err != nil {
		log.Println("invalid url", site.Key.SiteUrl(data), err)
		return
	}

	<-siteToken
	defer func() { siteToken <- true }()
	_, exists := data.Sites[siteKey]
	if exists && !force {
		return
	}
	if !exists {
		log.Println("    new link:", siteKey)
		sitesAddedThisRound++

		data.Sites[siteKey] = site
	}
	select {
	case queue <- data.Sites[siteKey]:
		queueSize.Add(1)
	default:
		log.Println("dropping", siteKey, "from queue - full.")
	}
}

func addRandomSite(siteKey SiteKey) {
	<-siteToken
	defer func() { siteToken <- true }()
	if _, exists := data.RandomSites[siteKey]; exists {
		return
	}
	log.Println("new randomSite", siteKey)
	data.RandomSites[siteKey] = true
}

func (w *listWriter) Write(b []byte) (int, error) {
	str := string(b)
	w.buffer.PushBack(str)
	for w.buffer.Len() > w.Height-2 {
		w.buffer.Remove(w.buffer.Front())
	}

	w.Items = w.Items[:0]
	for e := w.buffer.Front(); e != nil; e = e.Next() {
		w.Items = append(w.Items, e.Value.(string))
	}
	ui.Render(ui.Body)
	return len(b), nil
}

func (w *worker) run() {
	defer panicrecover()
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
	path += ".json"
	fmt.Println("loading savefile.")
	b, err := ioutil.ReadFile(path)
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		panic(errors.WithMessage(err, "loading savefile"))
	}
	fmt.Println("parsing savefile")
	err = json.Unmarshal(b, &data)
	if err != nil {
		panic(errors.WithMessage(err, "parsing savefile"))
	}
	fmt.Println("loaded.")
	return true
}
func save(path string) bool {
	<-siteToken

	path += ".json"
	log.Println("saving metadata to", path)
	b, err := json.MarshalIndent(&data, "", "  ")
	siteToken <- true
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

	siteUrl := data.RootUrl.String() + s.Key.Path
	w.log.Println("processing", siteUrl)

	err := w.fetchHTMLAndGrabLinks(siteUrl)
	if err != nil {
		w.log.Println(err)
	}

	// get history
	err = w.fetchHighestRevision(s)
	if err != nil {
		w.log.Println(err)
	}

	for _, rev := range s.Revisions {
		savePath := s.FileSavePath(rev.Revision, data)
		if _, err := os.Stat(savePath); os.IsNotExist(err) {
			w.log.Println("downloading", siteUrl, rev)
			w.download(s, rev)
		}
	}
}

func (w *worker) fetchHTMLAndGrabLinks(siteUrl string) error {
	d, err := goquery.NewDocument(siteUrl)
	if err != nil {
		return errors.Wrap(err, "error fetching document "+siteUrl)
	}
	// get HTML, find links
	d.Find("a,link").Each(w.parseAndAddLink)
	return nil
}

func (w *worker) parseAndAddLink(i int, e *goquery.Selection) {
	link, ok := e.Attr("href")
	if !ok {
		return
	}
	relativeUrl, err := url.Parse(link)
	if err != nil {
		w.log.Println("    invalid url", link, err)
		return
	}
	absoluteUrl := data.RootUrl.ResolveReference(relativeUrl)
	if !strings.HasPrefix(absoluteUrl.String(), data.RootUrl.String()+"/") {
		return
	}
	debug := true
	if strings.Contains(absoluteUrl.String(), "/Cultur%20Centrum%20Cassel?action=AttachFile") {
		debug = true
		w.log.Println("absolute:", absoluteUrl)
	}

	action := absoluteUrl.Query().Get("action")
	siteKey := SiteKey{Path: absoluteUrl.EscapedPath()}
	if debug {
		w.log.Println("action:", action, "query:", absoluteUrl.Query())
	}
	if action == "AttachFile" {
		target := absoluteUrl.Query().Get("target")
		if target == "" {
			return
		}
		target, err = url.QueryUnescape(target)
		if err != nil {
			w.log.Println("could not unescape target:", err)
			return
		}
		siteKey.Path += "/" + target
		siteKey.IsAttachment = true
	}
	if debug {
		w.log.Println("before trim&unescape:", siteKey)
	}
	siteKey.Path = strings.TrimPrefix(siteKey.Path, data.RootUrl.EscapedPath())
	siteKey.Path, err = url.QueryUnescape(siteKey.Path)
	if debug {
		w.log.Println("after trim&unescape:", siteKey)
	}
	if err != nil {
		w.log.Println("    invalid unescape", siteKey.Path, err)
		return
	}
	if strings.Contains(siteKey.Path, "Cultur%20Centrum%20Cassel?action=AttachFile&do=get&target=halle-plan.jpg") {
		w.doWaitStep()
	}

	addSite(siteKey, false)
}

func (w worker) fetchHighestRevision(s *Site) error {
	d, err := goquery.NewDocument(data.RootUrl.String() + s.Key.Path + "?action=info&max_count=9999")
	if err != nil {
		w.log.Println(err)
		w.doWaitStep()
		return errors.Wrap(err, "error fetching history "+s.Key.Path)
	}
	w.log.Println("fetched history", s.Key.Path+"?action=info&max_count=9999")
	// get HTML, find revisions
	s.HighestRevision = 1
	d.Find("tr").Each(func(_ int, el *goquery.Selection) {
		i, err := strconv.ParseInt(el.Find(".column0").First().Text(), 10, 64)
		rev := int(i)
		if err == nil && int(i) > s.HighestRevision {
			s.HighestRevision = rev
		} else if err != nil {
			return
		}
		if _, ok := s.Revisions[rev]; ok {
			return
		}
		s.Revisions[rev] = &Revision{Revision: rev}

		col1 := el.Find(".column1")
		if col1.Length() > 0 {
			s.Revisions[rev].Date, _ = time.Parse("2006-01-02 15:04:05", col1.Text())
		}
		col3 := el.Find(".column3")
		if col3.Length() > 0 {
			s.Revisions[rev].Size, _ = strconv.ParseInt(col3.Text(), 10, 64)
		}
		col4 := el.Find(".column4 span a")
		if col4.Length() > 0 {
			s.Revisions[rev].Author = col4.Text()
		} else {
			w.log.Println(goquery.OuterHtml(el.Find(".column4")))
			w.log.Println(goquery.OuterHtml(el.Find(".column4 span")))
			w.log.Println(goquery.OuterHtml(el.Find(".column4 span a")))
			//w.doWaitStep()
		}
		col5 := el.Find(".column5")
		if col5.Length() > 0 {
			s.Revisions[rev].Message = col5.Text()
		}
	})

	return nil
}

func (w *worker) download(s *Site, revision *Revision) {
	downloadUrl := fmt.Sprint(s.Key.RequestUrl(data), "&rev=", revision.Revision)
	w.log.Println("  escaped url: ", downloadUrl)

	req, err := http.NewRequest("GET", downloadUrl, nil)
	if err != nil {
		w.log.Println("  error:", err)
		w.doWaitStep()
		return
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
	if s.Key.IsAttachment && resp.Header.Get("Last-Modified") != "" {
		dateS = resp.Header.Get("Last-Modified")
		newDate, err := time.Parse(time.RFC1123, dateS)
		if err != nil {
			w.log.Println("error parsing last-modified:", err)
			w.doWaitStep()
			newDate = time.Now()
		}
		revision.Date = newDate
	}

	fileSavePath := s.FileSavePath(revision.Revision, data)

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		w.log.Println("  error reading response:", err)
		w.doWaitStep()
		return
	}
	revision.Size = int64(len(b))
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
	w.log.Println("  wrote", len(b), "bytes to", fileSavePath)

	if !s.Key.IsAttachment {
		bStr := string(b)
		if strings.Contains(bStr, "<<RandomPage") {
			s.HasRandom = true
			addRandomSite(s.Key)
		}
		matches := includeRegex.FindAllStringSubmatch(bStr, -1)
		for _, match := range matches {
			addSite(SiteKey{"/" + match[1], false}, false)
		}
	}

	buf := &bytes.Buffer{}
	resp.Header.Write(buf)
	s.Notes += buf.String() + "\n"
}

var mtypes = map[string]string{
	"text/plain":      ".txt",
	"text/html":       ".html",
	"image/png":       ".png",
	"image/jpeg":      ".jpeg",
	"image/gif":       ".gif",
	"application/pdf": ".pdf",
}

func (w *worker) getMimeAndExt(header http.Header) (string, string, error) {
	contentType := header.Get("Content-Type")
	mtype, _, err := mime.ParseMediaType(contentType)
	if err == nil {
		if e, ok := mtypes[mtype]; ok {
			return mtype, e, nil
		}
	} else {
		return "", "", err
	}
	w.log.Println("unknown mimetype", mtype)
	w.doWaitStep()
	return mtype, "", errors.New("unknown mimetype " + mtype)
}
