package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"time"

	"sort"

	"os"

	"github.com/cfstras/wiki-api/api"
	. "github.com/cfstras/wiki-api/types"
)

type History []HistoryEntry

type HistoryEntry struct {
	FilePath        string
	TargetPath      string
	Date            time.Time
	Author, Message string
}

func (h History) Len() int           { return len(h) }
func (h History) Less(i, j int) bool { return h[i].Date.Before(h[j].Date) }
func (h History) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

var data Data

func main() {
	var savePath, repoPath string
	var debug bool
	flag.StringVar(&savePath, "file", "", "savefile to work with (without .json)")
	flag.StringVar(&repoPath, "repo", "", "repo to import at")
	flag.BoolVar(&debug, "debug", false, "enable debug")
	flag.Parse()
	if savePath == "" || repoPath == "" {
		flag.Usage()
		return
	}
	if debug {
		go func() {
			log.Fatalln(http.ListenAndServe("localhost:6060", nil))
		}()
	}

	if !LoadData(savePath, &data) {
		log.Fatalln("Error: savefile " + savePath + ".json not found!")
		return
	}
	log.Println("building history...")
	history := buildHistory(&data)
	log.Println("starting import...")

	go func() {
		err := api.Run("127.0.0.1:3001", repoPath, false)
		if err != nil {
			log.Fatalln(err)
		}
	}()
	time.Sleep(1 * time.Second)

	//b, err := json.MarshalIndent(history, "", "  ")
	//fmt.Println(string(b), err)

	importHistory(history)
}

func buildHistory(data *Data) History {
	var history History
	for _, site := range data.Sites {
		log.Println("adding site", site.Key)
		for _, rev := range site.Revisions {
			entry := HistoryEntry{
				Author: rev.Author,
				Message: rev.Message + "\n" + site.Key.Path +
					" r" + fmt.Sprint(rev.Revision),
				FilePath:   site.FileSavePath(rev.Revision, data),
				TargetPath: site.Key.Path,
				Date:       rev.Date,
			}
			if !site.Key.IsAttachment {
				entry.TargetPath += ".md"
			}
			//log.Println("appending", entry)
			history = append(history, entry)
		}
	}
	sort.Sort(history)
	return history
}

func importHistory(hist History) {
	for _, e := range hist {
		body, err := os.Open(e.FilePath)
		log.Println("importing", e.TargetPath, "@", e.Date)
		if err != nil {
			log.Println("opening", e.FilePath, ":", err, ". Skipping.")
			continue
		}
		err, code := api.PutFile(e.TargetPath, "", e.Message, body)
		if code != http.StatusOK {
			log.Fatalln("importing", e.TargetPath, ":", code, err)
		}
	}
}
