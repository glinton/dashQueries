// todo:
// - write dashboard file as soon as it's got to avoid loss on app close/error
// - skip if dashboard file exists (to continue working from where quit)
// - parallelize requests for at least all cells on a board

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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
)

func init() {
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

	queries := getAllQueries(gBoards)
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	err = writeQueries(queries)
	if err != nil {
		fmt.Println(err.Error())
		fmt.Printf("\n%+v\n", queries)
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

func getAllQueries(dashboards []dash) []dash {
	for i := range dashboards {
		queries, err := getDashboardCellQueries(dashboards[i])
		if err != nil {
			fmt.Printf("failed to get dashboard cell queries for %q: %s\n", dashboards[i].ID, err.Error())
			continue
		}

		dashboards[i].QueryTexts = queries
		dashboards[i].Cells = nil
	}

	return dashboards
}

func getDashboardCellQueries(dashboard dash) ([]string, error) {
	queries := []string{}

	for i := range dashboard.Cells {
		cellQueries, err := getCellQueries(dashboard.Cells[i])
		if err != nil {
			fmt.Printf("failed to get cell queries for %q: %s\n", dashboard.Cells[i].Links.View, err.Error())
			continue
		}

		queries = append(queries, cellQueries...)
	}

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

func writeQueries(dashboards []dash) error {
	destDir := filepath.Join(".", "dashboards")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination dir: %s", err.Error())
	}

	for i := range dashboards {
		f, err := os.Create(filepath.Join(destDir, dashboards[i].ID+".json"))
		if err != nil {
			return fmt.Errorf("failed to open file to write: %s", err.Error())
		}

		err = json.NewEncoder(f).Encode(dashboards[i])
		if err != nil {
			fmt.Printf("failed to write dashboard %q to file: %s\n", dashboards[i].ID, err.Error())
			continue
		}
		// strings.Join([]string{".", "dashboards", dashboards[i].ID}, string(os.PathSeparator))
	}

	return nil
}
