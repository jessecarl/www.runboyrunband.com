// Copyright 2013 Jesse Allen. All rights reserved
// Released under the MIT license found in the LICENSE file.

package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"

	// TODO [jesse@jessecarl.com]: Move this dependency to a public repo
	"runboyrunband.com/www/shows"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	httpgzip "github.com/daaku/go.httpgzip"
	"github.com/lazyengineering/gobase/envflag"
	"github.com/lazyengineering/gobase/layouts"
	"github.com/lazyengineering/gobase/layouts/filters"
	"github.com/lazyengineering/gobase/redirect"
)

// Important metadata
var (
	ServerAddr   = flag.String("server-addr", ":5050", "Server Address to listen on")
	GATrackingID = flag.String("ga-tracking-id", "", "Google Analytics Tracking ID")
)

var Layout *layouts.Layout

func init() {
	t := time.Now() // measure bootstrap time
	defer func() {
		log.Printf("\x1b[1;32mBootstrapped:\x1b[0m \x1b[34m%8d\x1b[0mµs", time.Since(t).Nanoseconds()/1000)
	}()
	var (
		NoTimestamp        = flag.Bool("no-timestamp", false, "When set to true, removes timestamp from log statements")
		StaticDir          = flag.String("static-dir", "static", "Static Assets folder")
		LayoutTemplateGlob = flag.String("layouts", "static/templates/layouts/*.html", "Pattern for layout templates")
		HelperTemplateGlob = flag.String("helpers", "static/templates/helpers/*.html", "Pattern for helper templates")
		SongkickArtistID   = flag.Int("songkick-artist-id", 0, "Songkick Artist ID")
		SongkickApiKey     = flag.String("songkick-api-key", "", "Songkick API Key")
		DataBucket         = flag.String("data-bucket", "", "AWS Bucket where data resides")
		DataKeyPrefix      = flag.String("data-key-prefix", "", "Prefix for all AWS keys in data bucket")
	)

	// To Parse flags, looking for command-line, then ENV, then defaults
	envflag.Parse(envflag.FlagMap{
		"server-addr": envflag.Flag{
			Name:   "PORT",
			Filter: func(s string) string { return ":" + s },
		},
	})

	if *NoTimestamp {
		log.SetFlags(0)
	}

	// Static Asset Serving
	staticServer := NoIndex(func(h http.Handler) http.Handler {
		// add 1 day caching headers to static assets
		return http.HandlerFunc(func(r http.ResponseWriter, q *http.Request) {
			r.Header().Set("Cache-Control", "public, max-age=86400")
			r.Header().Set("Expires", time.Now().Add(24*time.Hour).Format(time.RFC1123))
			h.ServeHTTP(r, q)
		})
	}(http.FileServer(http.Dir(*StaticDir))))
	Handle("/js/", staticServer)
	Handle("/css/", staticServer)
	Handle("/fonts/", staticServer)
	Handle("/img/", staticServer)
	Handle("/favicon.ico", staticServer)
	Handle("/robots.txt", staticServer)

	// Offsite Redirects
	http.Handle("/e/", http.StripPrefix("/e/", redirect.ServePermanentRedirects(func() map[string]string {
		m := make(map[string]string)
		j, err := readFromS3(*DataBucket, *DataKeyPrefix+"redirects.json")
		if err != nil {
			// because we're still in bootstrap
			panic(err)
		}
		err = json.Unmarshal(j, &m)
		if err != nil {
			// because we're still in bootstrap
			panic(err)
		}
		return m
	}())))

	{
		// Layouts
		var err error
		f := filters.All
		f["Now"] = time.Now
		Layout, err = layouts.New(filters.All, "bootstrap.html", *LayoutTemplateGlob, *HelperTemplateGlob)
		if err != nil {
			// fatal condition
			panic(err)
		}
	}

	// Actual Web Application Handlers
	{
		HandleNoSubPaths("/", Layout.Act(layouts.MergeActions(
			basicData,
			staticData(map[string]interface{}{
				"Title":     "Run Boy Run",
				"BodyClass": "home",
			}),
			teaserData(*DataBucket, *DataKeyPrefix),
			bigNewsData(*DataBucket, *DataKeyPrefix),
		), Error500, layouts.LowVolatility, "static/templates/home/*.html"))
		HandleNoSubPaths("/music/", Layout.Act(layouts.MergeActions(
			basicData,
			staticData(map[string]interface{}{"Title": "Run Boy Run – Music"}),
			musicData(*DataBucket, *DataKeyPrefix),
		), Error500, layouts.LowVolatility, "static/templates/music/*.html"))
		HandleNoSubPaths("/shows/", Layout.Act(layouts.MergeActions(
			basicData,
			staticData(map[string]interface{}{"Title": "Run Boy Run – Shows"}),
			showsData(*SongkickArtistID, *SongkickApiKey),
		), Error500, layouts.LowVolatility, "static/templates/shows/*.html"))
		HandleNoSubPaths("/about/", Layout.Act(layouts.MergeActions(
			basicData,
			staticData(map[string]interface{}{"Title": "Run Boy Run – About"}),
			bioData(*DataBucket, *DataKeyPrefix),
			quoteData(*DataBucket, *DataKeyPrefix),
			headshotData(*DataBucket, *DataKeyPrefix),
		), Error500, layouts.LowVolatility, "static/templates/about/*.html"))
		HandleNoSubPaths("/contact/", Layout.Act(layouts.MergeActions(
			basicData,
			staticData(map[string]interface{}{"Title": "Run Boy Run – Contact"}),
			contactData(*DataBucket, *DataKeyPrefix),
		), Error500, layouts.LowVolatility, "static/templates/contact/*.html"))
		HandleNoSubPaths("/photos/", Layout.Act(layouts.MergeActions(
			basicData,
			staticData(map[string]interface{}{"Title": "Run Boy Run – Photos"}),
			photosData(*DataBucket, *DataKeyPrefix),
		), Error500, layouts.LowVolatility, "static/templates/photos/*.html"))
		HandleNoSubPaths("/videos/", Layout.Act(layouts.MergeActions(
			basicData,
			staticData(map[string]interface{}{"Title": "Run Boy Run – Videos"}),
			videosData,
		), Error500, layouts.LowVolatility, "static/templates/videos/*.html"))
	}
}

// Log and Handle http requests
func Handle(path string, h http.Handler) {
	if strings.HasSuffix(path, "/") { // redirect for directories
		indexRedirect := http.RedirectHandler(path, http.StatusMovedPermanently)
		Handle(path+"index.html", indexRedirect)
		Handle(path+"index.htm", indexRedirect)
		Handle(path+"index.php", indexRedirect) // not that anybody would think...
	}
	http.Handle(path, httpgzip.NewHandler(http.HandlerFunc(func(r http.ResponseWriter, q *http.Request) {
		h.ServeHTTP(r, q)
	})))
}

func HandleFunc(path string, h http.HandlerFunc) {
	Handle(path, http.HandlerFunc(h))
}

func HandleNoSubPaths(path string, h http.Handler) {
	Handle(path, NoSubPaths(path, h))
}

func NoIndex(h http.Handler) http.Handler {
	return http.HandlerFunc(func(r http.ResponseWriter, q *http.Request) {
		if strings.HasSuffix(q.URL.Path, "/") {
			Error403(r, q)
			return
		}
		h.ServeHTTP(r, q)
	})
}

func NoSubPaths(path string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(r http.ResponseWriter, q *http.Request) {
		if q.URL.Path != path {
			Error404(r, q)
			return
		}
		h.ServeHTTP(r, q)
	})
}

func main() {
	log.Println("\x1b[32mlistening at \x1b[1;32m" + *ServerAddr + "\x1b[32m...\x1b[0m")
	log.Fatalln("Fatal Error:", http.ListenAndServe(*ServerAddr, nil))
}

type Nav struct {
	*http.Request
}

func (n Nav) IsCurrent(p string) bool {
	return p == n.Request.URL.Path
}

func staticData(data map[string]interface{}) layouts.Action {
	return func(req *http.Request) (map[string]interface{}, error) {
		return data, nil
	}
}

func basicData(req *http.Request) (map[string]interface{}, error) {
	return map[string]interface{}{
		"Nav":          Nav{req},
		"GATrackingID": *GATrackingID,
	}, nil
}

func readFromS3(bucket, key string) ([]byte, error) {
	// assume we don't need multi-part downloads for this kind of data
	t := time.Now()
	defer func() {
		log.Printf("\x1b[1;35mGetObject:\x1b[0m \x1b[34m%12d\x1b[0mµs \x1b[33m%s\x1b[0m", time.Since(t)/1000, key)
	}()
	svc := s3.New(session.New())
	resp, err := svc.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	return ioutil.ReadAll(resp.Body)
}

func teaserData(bucket, prefix string) layouts.Action {
	return func(req *http.Request) (map[string]interface{}, error) {
		teaser, err := readFromS3(bucket, prefix+"teaser.md")
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"Teaser": string(teaser),
		}, nil
	}
}

func bigNewsData(bucket, prefix string) layouts.Action {
	return func(req *http.Request) (map[string]interface{}, error) {
		// big news items are essentially fliers that link out to something important
		type newsItem struct {
			Image, IFrame, Alt             string
			URL, CallToAction, Description string
			Landscape                      bool // indicate if flier is landscape orientation
			Expires                        time.Time
		}
		var bigNewsJson []byte
		var bigNews []newsItem
		var err error
		bigNewsJson, err = readFromS3(bucket, prefix+"big-news.json")
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(bigNewsJson, &bigNews)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"BigNews": bigNews,
			"ExtraJS": []string{"/js/big-news.js"},
		}, nil
	}
}

func headshotData(bucket, prefix string) layouts.Action {
	return func(req *http.Request) (map[string]interface{}, error) {
		type headshot struct{ Name, Image, Looking, Plays string }
		var headshots []headshot
		var headshotJson []byte
		var err error
		headshotJson, err = readFromS3(bucket, prefix+"headshots.json")
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(headshotJson, &headshots)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"Headshots": headshots,
		}, nil
	}
}

func bioData(bucket, prefix string) layouts.Action {
	return func(req *http.Request) (map[string]interface{}, error) {
		var bio []byte
		var err error
		// read bio from markdown file
		bio, err = readFromS3(bucket, prefix+"bio.md")
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"Bio": string(bio),
		}, nil
	}
}

func quoteData(bucket, prefix string) layouts.Action {
	return func(req *http.Request) (map[string]interface{}, error) {
		type quote struct {
			Quote       string
			Attribution struct{ Name, URL, Affiliation string }
		}
		var quotes []quote
		var quoteJson []byte
		var err error
		quoteJson, err = readFromS3(bucket, prefix+"quotes.json")
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(quoteJson, &quotes)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"Quotes": quotes,
		}, nil
	}
}

func musicData(bucket, prefix string) layouts.Action {
	return func(req *http.Request) (map[string]interface{}, error) {
		type quote struct {
			Quote       string
			Attribution struct{ Name, URL, Affiliation string }
		}
		type Album struct {
			Name          string
			Url           string
			Image         string // img src
			DatePublished time.Time
			Description   string
			BandcampID    string
			Endorsement   []quote
		}
		albumJson, err := readFromS3(bucket, prefix+"albums.json")
		if err != nil {
			return nil, err
		}
		var albums []Album
		err = json.Unmarshal(albumJson, &albums)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"Albums": albums,
		}, nil
	}
}

func showsData(artistID int, apiKey string) layouts.Action {
	c := shows.New(artistID, apiKey)
	return func(req *http.Request) (map[string]interface{}, error) {
		// Load Shows from API
		past, err := c.Past(0)
		if err != nil {
			Error200(req, err)
		}
		return map[string]interface{}{
			"Events": struct {
				Past []shows.Event
			}{
				past,
			},
		}, nil
	}
}

func contactData(bucket, prefix string) layouts.Action {
	return func(req *http.Request) (map[string]interface{}, error) {
		type contact struct {
			Realm, Name, Email, Telephone string
			LinkEmail                     bool
			Affiliation                   struct {
				Name, URL string
			}
		}
		var contacts []struct {
			Realm   string
			Contact []contact
		}
		if contactsJson, err := readFromS3(bucket, prefix+"contact.json"); err != nil {
			return nil, err

		} else {
			err = json.Unmarshal(contactsJson, &contacts)
			if err != nil {
				return nil, err
			}
		}
		return map[string]interface{}{
			"Contacts": contacts,
		}, nil
	}
}

func photosData(bucket, prefix string) layouts.Action {
	return func(req *http.Request) (map[string]interface{}, error) {
		type photo struct{ Image, Copyright, Orientation, Composition string }
		var photos []photo
		if photosJson, err := readFromS3(bucket, prefix+"photos.json"); err != nil {
			return nil, err

		} else {
			err = json.Unmarshal(photosJson, &photos)
			if err != nil {
				return nil, err
			}
		}
		// TODO: Move "Run Boy Run" out of titles into templates
		return map[string]interface{}{
			"Photos":  photos,
			"ExtraJS": []string{"/js/photos.js"},
		}, nil
	}
}

func videosData(req *http.Request) (map[string]interface{}, error) {
	return map[string]interface{}{
		"ExtraJS": []string{
			"//ajax.googleapis.com/ajax/libs/swfobject/2.2/swfobject.js",
			"/js/videos.js",
		},
	}, nil
}
