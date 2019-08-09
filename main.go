package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
)

type glProject struct {
	ID                uint64 `json:"id"`
	PathWithNamespace string `json:"path_with_namespace"`
}

type glBranch struct {
	Commit struct {
		ID string `json:"id"`
	} `json:"commit"`
}

type glCommit struct {
	LastPipeline struct {
		ID uint64 `json:"id"`
	} `json:"last_pipeline"`
}

type glJob struct {
	ID        uint64     `json:"id"`
	Stage     string     `json:"stage"`
	Artifacts []struct{} `json:"artifacts"`
}

var client http.Client
var token string

func main() {
	baseUrl := flag.String("baseurl", "", "https://gitlab.example.com/")
	project := flag.String("project", "", "diaspora/diaspora-client")
	stage := flag.String("stage", "", "test")
	flag.Usage = usage

	flag.Parse()

	branches := flag.Args()

	if *baseUrl == "" {
		fmt.Fprintln(os.Stderr, "base URL missing")
		usage()
		os.Exit(2)
	}

	if *project == "" {
		fmt.Fprintln(os.Stderr, "project missing")
		usage()
		os.Exit(2)
	}

	if *stage == "" {
		fmt.Fprintln(os.Stderr, "stage missing")
		usage()
		os.Exit(2)
	}

	if len(branches) < 1 {
		fmt.Fprintln(os.Stderr, "branches missing")
		usage()
		os.Exit(2)
	}

	token = os.Getenv("TOKEN")
	if token == "" {
		fmt.Fprintln(os.Stderr, "token missing")
		usage()
		os.Exit(2)
	}

	rootURL, errURL := url.Parse(*baseUrl)
	if errURL != nil {
		fmt.Fprintf(os.Stderr, "bad base URL: %s\n", errURL.Error())
		usage()
		os.Exit(2)
	}

	for _, branch := range branches {
		if branch == "" {
			fmt.Fprintf(os.Stderr, "bad branch: %s\n", branch)
			usage()
			os.Exit(2)
		}
	}

	if !strings.HasSuffix(rootURL.Path, "/") {
		rootURL.Path += "/"
		rootURL.RawPath += "/"
	}

	apiURL := rootURL.ResolveReference(parseURL("api/v4/"))

	var projectID uint64

	{
		projectsURL := apiURL.ResolveReference(parseURL("projects"))
		var page uint64 = 1

	PaginatingProjects:
		for {
			projectsURL.RawQuery = fmt.Sprintf("page=%d", page)

			var projects []glProject
			assert(getJson(projectsURL, &projects))

			if len(projects) < 1 {
				assert(errors.New("no such project"))
			}

			for i := range projects {
				if projct := &projects[i]; projct.PathWithNamespace == *project {
					projectID = projct.ID
					break PaginatingProjects
				}
			}

			page++
		}
	}

	branchesURL := apiURL.ResolveReference(parseURL(fmt.Sprintf("projects/%d/repository/branches/", projectID)))
	commitsURL := apiURL.ResolveReference(parseURL(fmt.Sprintf("projects/%d/repository/commits/", projectID)))
	pipelinesURL := apiURL.ResolveReference(parseURL(fmt.Sprintf("projects/%d/pipelines/", projectID)))
	jobsURL := apiURL.ResolveReference(parseURL(fmt.Sprintf("projects/%d/jobs/", projectID)))

	for _, branch := range branches {
		var brnch glBranch
		assert(getJson(branchesURL.ResolveReference(parseURL(url.PathEscape(branch))), &brnch))

		var commit glCommit
		assert(getJson(commitsURL.ResolveReference(parseURL(url.PathEscape(brnch.Commit.ID))), &commit))

		var jobs []glJob
		assert(getJson(pipelinesURL.ResolveReference(parseURL(fmt.Sprintf("%d/jobs", commit.LastPipeline.ID))), &jobs))

		for i := range jobs {
			if job := &jobs[i]; job.Stage == *stage && len(job.Artifacts) > 0 {
				f, errOpen := os.Create(fmt.Sprintf("%s-%d.zip", url.PathEscape(branch), job.ID))
				assert(errOpen)

				res, errReq := client.Do(&http.Request{
					Method: "GET",
					URL:    jobsURL.ResolveReference(parseURL(fmt.Sprintf("%d/artifacts", job.ID))),
					Header: map[string][]string{"Private-Token": {token}},
				})
				assert(errReq)

				_, errCopy := io.Copy(f, res.Body)
				assert(errCopy)

				f.Close()
				res.Body.Close()
			}
		}
	}
}

func parseURL(uri string) *url.URL {
	urAL, errURL := url.Parse(uri)
	if errURL != nil {
		panic(errURL)
	}

	return urAL
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: TOKEN=123456 %s -baseurl https://gitlab.example.com/ -project diaspora/diaspora-client -stage test branch1 [branches2toN...]\n", os.Args[0])
}

func assert(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func getJson(urAL *url.URL, jsn interface{}) error {
	res, errReq := client.Do(&http.Request{Method: "GET", URL: urAL, Header: map[string][]string{"Private-Token": {token}}})
	if errReq != nil {
		return errReq
	}

	defer res.Body.Close()

	body, errRA := ioutil.ReadAll(res.Body)
	if errRA != nil {
		return errRA
	}

	return json.Unmarshal(body, jsn)
}
