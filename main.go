package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

type link struct {
	View string `json:"view"`
}

type dashCell struct {
	Links link `json:"links"`
}

type dash struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Cells      []dashCell `json:"cells,omitempty"`
	QueryTexts []string   `json:"queries"`
}

var (
	upstream string
	cookie   string
	limit    int
	destDir  string
)

func init() {
	flag.StringVar(&destDir, "d", filepath.Join(".", "dashboards"), "Location to store dashboards.")
	flag.StringVar(&upstream, "u", "", "Upstream host to query.")
	flag.StringVar(&cookie, "c", "", "Cookie for request.")
	flag.IntVar(&limit, "l", -1, "Limit to number of dashboards.")
	flag.Parse()

	if upstream == "" {
		fmt.Println("upstream must be set")
		os.Exit(1)
	}
	if cookie == "" {
		fmt.Println("cookie must be set")
		os.Exit(1)
	}
}

func main() {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		fmt.Printf("failed to create destination dir: %s\n", err.Error())
		return
	}

	dashboards, err := getDashboards(upstream, cookie)
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	if len(dashboards) == 0 {
		fmt.Println("no dashboards returned")
		return
	}

	gBoards := dashboards
	if limit > 0 {
		gBoards = dashboards[:limit]
	}

	getAllQueries(gBoards)
	if err != nil {
		fmt.Println(err.Error())
		return
	}
}

func getDashboards(upstream, cookie string) ([]dash, error) {
	type dlist struct {
		Boards []dash `json:"dashboards"`
	}

	r, err := http.NewRequest("GET", upstream+"/api/v2/dashboards", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %s", err.Error())
	}
	r.Header.Add("cookie", cookie)

	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		return nil, fmt.Errorf("failed to get dashboards: %s", err.Error())
	}

	listed := &dlist{}
	if err := json.NewDecoder(resp.Body).Decode(listed); err != nil {
		return nil, fmt.Errorf("failed to decode dashboards: %s", err.Error())
	}

	return listed.Boards, nil
}

func getAllQueries(dashboards []dash) {
	for i := range dashboards {
		getAllDashboardQueries(&dashboards[i])
	}
}

func getAllDashboardQueries(dashboard *dash) {
	if fileExists(filepath.Join(destDir, dashboard.ID+".json")) {
		return
	}

	queries, err := getDashboardCellQueries(*dashboard)
	if err != nil {
		fmt.Printf("failed to get dashboard cell queries for %q: %s\n", dashboard.ID, err.Error())
		return
	}

	dashboard.QueryTexts = queries
	dashboard.Cells = nil

	writeQuery(*dashboard)
}

func getDashboardCellQueries(dashboard dash) ([]string, error) {
	queries := []string{}
	qtex := &sync.Mutex{}
	wg := &sync.WaitGroup{}

	for i := range dashboard.Cells {
		wg.Add(1)

		go func(cells dashCell) {
			defer wg.Done()

			cellQueries, err := getCellQueries(cells)
			if err != nil {
				fmt.Printf("failed to get cell queries for %q: %s\n", dashboard.Cells[i].Links.View, err.Error())
				return
			}

			qtex.Lock()
			queries = append(queries, cellQueries...)
			qtex.Unlock()
		}(dashboard.Cells[i])
	}

	// wait for cells to be populated
	wg.Wait()
	return queries, nil
}

func getCellQueries(dCell dashCell) ([]string, error) {
	type cell struct {
		Properties struct {
			Queries []struct {
				Text string `json:"text"`
			} `json:"queries"`
		} `json:"properties"`
	}

	r, err := http.NewRequest("GET", upstream+dCell.Links.View, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %s", err.Error())
	}
	r.Header.Add("cookie", cookie)
	r.Header.Add("user-agent", "cell getter 3000")

	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		return nil, fmt.Errorf("failed to get cells: %s", err.Error())
	}

	listed := &cell{}
	if err := json.NewDecoder(resp.Body).Decode(listed); err != nil {
		return nil, fmt.Errorf("failed to decode cells: %s", err.Error())
	}

	queries := []string{}
	for _, query := range listed.Properties.Queries {
		queries = append(queries, query.Text)
	}

	return queries, nil
}

func fileExists(filename string) bool {
	_, err := os.Stat(filename)

	return err == nil
}

func writeQuery(dashboard dash) {
	f, err := os.Create(filepath.Join(destDir, dashboard.ID+".json"))
	if err != nil {
		fmt.Printf("failed to open file to write: %s\n", err.Error())
		return
	}

	err = json.NewEncoder(f).Encode(dashboard)
	if err != nil {
		fmt.Printf("failed to write dashboard %q to file: %s\n", dashboard.ID, err.Error())
	}
}
