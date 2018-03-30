// codehn is a hn clone that only displays posts from github
package main

// lots of imports means lots of time saved
import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	// for "33 minutes ago" in the template
	humanize "github.com/dustin/go-humanize"

	// because the HN API is awkward and slow
	cache "github.com/pmylund/go-cache"
)

// baseURL is the URL for the hacker news API
var baseURL = "https://hacker-news.firebaseio.com/v0/"

// cash rules everything around me, get the money y'all
var cash *cache.Cache

// we will ensure the template is valid and only load
var tmpl *template.Template

func init() {

	// cash will have default expiration time of
	// 30 minutes and be swept every 10 minutes
	cash = cache.New(30*time.Minute, 10*time.Minute)

	// this will panic if the index.tmpl isn't valid
	tmpl = template.Must(template.ParseFiles("index.tmpl"))
}

// story holds the response from the HN API and
// two other fields I use to render the template
type story struct {
	By          string `json:"by"`
	Descendants int    `json:"descendants"`
	ID          int    `json:"id"`
	Kids        []int  `json:"kids"`
	Score       int    `json:"score"`
	Time        int    `json:"time"`
	Title       string `json:"title"`
	Type        string `json:"type"`
	URL         string `json:"url"`
	DomainName  string
	HumanTime   string
}

// stories is just a bunch of story pointers
type stories []*story

// getStories if you couldn't guess it, gets the stories
func getStories(res *http.Response) (stories, error) {

	// this is bad! we should limit the request body size
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	// get all the story keys into a slice of ints
	var keys []int
	json.Unmarshal(body, &keys)

	// concurrency is cool, but needs to be limited
	semaphore := make(chan struct{}, 10)

	// how we know when all our goroutines are done
	wg := sync.WaitGroup{}

	// somewhere to store all the stories when we're done
	var stories []*story

	// go over all the stories
	for _, key := range keys {

		// stop when we have 30 stories
		if len(stories) >= 30 {
			break
		}

		// sleep to avoid rate limiting from API
		time.Sleep(10 * time.Millisecond)

		// in a goroutine with the story key
		go func(storyKey int) {

			// add one to the wait group
			wg.Add(1)

			// add one to the semaphore
			semaphore <- struct{}{}

			// make sure this gets fired
			defer func() {

				// remove one from the wait group
				wg.Done()

				// remove one from the semaphore
				<-semaphore
			}()

			// get the story with reckless abandon for errors
			keyURL := fmt.Sprintf(baseURL+"v0/item/%d.json", storyKey)
			res, err := http.Get(keyURL)
			if err != nil {
				return
			}
			defer res.Body.Close()

			body, err = ioutil.ReadAll(res.Body)
			if err != nil {
				return
			}

			s := &story{}
			err = json.Unmarshal(body, s)
			if err != nil {
				return
			}

			// check if it's from github or gitlab before adding to stories
			if strings.Contains(s.URL, "github") || strings.Contains(s.URL, "gitlab") {
				s.HumanTime = humanize.Time(time.Unix(int64(s.Time), 0))
				hostName, err := url.Parse(s.URL)
				if err == nil {
					s.DomainName = hostName.Hostname()
				}
				stories = append(stories, s)
			}

		}(key)
	}

	// wait for all the goroutines
	wg.Wait()

	return stories, nil
}

// getStoriesFromType gets the different types of stories the API exposes
func getStoriesFromType(pageType string) (stories, error) {
	var url string
	switch pageType {
	case "best":
		url = baseURL + "beststories.json"
	case "new":
		url = baseURL + "newstories.json"
	case "show":
		url = baseURL + "showstories.json"
	default:
		url = baseURL + "topstories.json"
	}

	res, err := http.Get(url)
	if err != nil {
		return nil, errors.New("could not get " + pageType + " hacker news posts list")
	}

	defer res.Body.Close()
	s, err := getStories(res)
	if err != nil {
		return nil, errors.New("could not get " + pageType + " hacker news posts data")
	}

	return s, nil
}

// container holds data used by the template
type container struct {
	Page    string
	Stories stories
}

// pageHandler returns a handler for the correct page type
func pageHandler(pageType string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {

		// we'll get all the stories
		var s stories

		// only because of shadowing
		var err error

		// know if we should use the cache
		var ok bool

		// check if we hit the cached stories for this page type
		x, found := cash.Get(pageType)
		if found {

			// check if valid stories
			s, ok = x.(stories)
		}

		// if it's not or we didn't hit the cached stories
		if !ok {

			// get the stories from the API
			s, err = getStoriesFromType(pageType)
			if err != nil {
				w.WriteHeader(500)
				w.Write([]byte(err.Error()))
				return
			}

			// set the cached stories for this page type
			cash.Set(pageType, s, cache.DefaultExpiration)
		}

		// parse the template file and return an error if it's broken
		tmpl, err := template.ParseFiles("index.tmpl")
		if err != nil {
			w.WriteHeader(500)
			w.Write([]byte("could not parse template file"))
			return
		}

		// finally let's just return 200 and write the template out
		w.WriteHeader(200)

		// set the content type header with html and utf encoding
		w.Header().Set("Content-type", "text/html;charset=utf-8")

		// execute the template which writes to w and uses container input
		tmpl.Execute(w, container{
			Stories: s,
			Page:    pageType,
		})
	}
}

// the main attraction, what you've all been waiting for
func main() {

	// port 8080 is a good choice
	port := ":8080"

	// set up our routes and handlers
	http.HandleFunc("/", pageHandler("top"))
	http.HandleFunc("/new", pageHandler("new"))
	http.HandleFunc("/show", pageHandler("show"))
	http.HandleFunc("/best", pageHandler("best"))

	// start the server up on our port
	log.Printf("Running on %s\n", port)
	log.Fatalln(http.ListenAndServe(port, nil))
}
