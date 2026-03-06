package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"math"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"
)

// --- lorem ipsum generator ---

var loremWords = []string{
	"lorem", "ipsum", "dolor", "sit", "amet", "consectetur", "adipiscing", "elit",
	"sed", "do", "eiusmod", "tempor", "incididunt", "ut", "labore", "et", "dolore",
	"magna", "aliqua", "enim", "ad", "minim", "veniam", "quis", "nostrud",
	"exercitation", "ullamco", "laboris", "nisi", "aliquip", "ex", "ea", "commodo",
	"consequat", "duis", "aute", "irure", "in", "reprehenderit", "voluptate",
	"velit", "esse", "cillum", "fugiat", "nulla", "pariatur", "excepteur", "sint",
	"occaecat", "cupidatat", "non", "proident", "sunt", "culpa", "qui", "officia",
	"deserunt", "mollit", "anim", "id", "est", "laborum", "praesentium", "voluptatum",
	"deleniti", "atque", "corrupti", "quos", "dolores", "quas", "molestias",
	"excepturi", "obcaecati", "cupiditate", "provident", "similique", "accusantium",
	"nemo", "ipsam", "voluptatem", "quia", "voluptas", "aspernatur", "aut", "odit",
	"fugit", "consequuntur", "magni", "ratione", "sequi", "nesciunt", "neque",
	"porro", "quisquam", "numquam", "eius", "modi", "tempora", "quaerat",
}

var tagPool = []string{
	"golang", "tutorial", "devops", "kubernetes", "docker", "react", "typescript",
	"database", "api", "microservices", "testing", "performance", "security",
	"cloud", "linux", "architecture", "monitoring", "ci-cd", "rust", "python",
}

func randomWord() string {
	return loremWords[rand.Intn(len(loremWords))]
}

func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func randomSentence() string {
	n := 8 + rand.Intn(8)
	words := make([]string, n)
	for i := range words {
		words[i] = randomWord()
	}
	words[0] = capitalize(words[0])
	return strings.Join(words, " ") + "."
}

func randomParagraph() string {
	n := 3 + rand.Intn(4)
	sentences := make([]string, n)
	for i := range sentences {
		sentences[i] = randomSentence()
	}
	return strings.Join(sentences, " ")
}

func randomTitle() string {
	n := 3 + rand.Intn(4)
	words := make([]string, n)
	for i := range words {
		words[i] = capitalize(randomWord())
	}
	return strings.Join(words, " ")
}

func randomTags() []string {
	n := 1 + rand.Intn(3)
	tags := make([]string, n)
	for i := range tags {
		tags[i] = tagPool[rand.Intn(len(tagPool))]
	}
	return tags
}

func randomBlogPost() (title string, content string, tags []string) {
	title = randomTitle()
	nParagraphs := 2 + rand.Intn(4)
	paragraphs := make([]string, nParagraphs)
	for i := range paragraphs {
		paragraphs[i] = randomParagraph()
	}
	content = "# " + title + "\n\n" + strings.Join(paragraphs, "\n\n")
	tags = randomTags()
	return
}

// --- MDDB client ---

type addRequest struct {
	Collection string              `json:"collection"`
	Key        string              `json:"key"`
	Lang       string              `json:"lang"`
	Meta       map[string][]string `json:"meta"`
	ContentMD  string              `json:"contentMd"`
}

func addDoc(client *http.Client, baseURL, collection string, docNum int) error {
	title, content, tags := randomBlogPost()
	key := fmt.Sprintf("post-%d", docNum)

	body, _ := json.Marshal(addRequest{
		Collection: collection,
		Key:        key,
		Lang:       "en",
		Meta: map[string][]string{
			"title":  {title},
			"tags":   tags,
			"author": {fmt.Sprintf("author-%d", rand.Intn(20))},
		},
		ContentMD: content,
	})

	resp, err := client.Post(baseURL+"/v1/add", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("POST /v1/add: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("POST /v1/add: status %d", resp.StatusCode)
	}
	return nil
}

func checkConnectivity(client *http.Client, baseURL string) error {
	resp, err := client.Get(baseURL + "/v1/stats")
	if err != nil {
		return fmt.Errorf("cannot connect to %s: %w", baseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET /v1/stats returned %d", resp.StatusCode)
	}
	return nil
}

func deleteCollection(client *http.Client, baseURL, collection string) error {
	body, _ := json.Marshal(map[string]string{"collection": collection})
	resp, err := client.Post(baseURL+"/v1/delete-collection", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// --- batch result ---

type batchResult struct {
	BatchNum   int
	DocsTotal  int
	Duration   time.Duration
	Throughput float64 // docs/sec for this batch
	CumAvg     float64 // cumulative average docs/sec
}

// --- main ---

func main() {
	url := flag.String("url", "http://localhost:7890", "MDDB base URL")
	collection := flag.String("collection", "bench", "Collection name")
	total := flag.Int("total", 10000, "Total documents to insert")
	batch := flag.Int("batch", 100, "Batch size for timing")
	output := flag.String("output", "bench_report.html", "HTML report output path")
	cleanup := flag.Bool("cleanup", false, "Delete collection after benchmark")
	flag.Parse()

	client := &http.Client{Timeout: 30 * time.Second}

	fmt.Printf("MDDB Benchmark\n")
	fmt.Printf("  URL:        %s\n", *url)
	fmt.Printf("  Collection: %s\n", *collection)
	fmt.Printf("  Total:      %d docs\n", *total)
	fmt.Printf("  Batch:      %d docs\n", *batch)
	fmt.Println()

	// Check connectivity
	if err := checkConnectivity(client, *url); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Connected to MDDB.")
	fmt.Println()

	nBatches := (*total + *batch - 1) / *batch
	results := make([]batchResult, 0, nBatches)
	totalDocs := 0
	var totalElapsed time.Duration

	for b := 0; b < nBatches; b++ {
		batchSize := *batch
		if totalDocs+batchSize > *total {
			batchSize = *total - totalDocs
		}

		start := time.Now()
		for i := 0; i < batchSize; i++ {
			docNum := totalDocs + i + 1
			if err := addDoc(client, *url, *collection, docNum); err != nil {
				fmt.Fprintf(os.Stderr, "\nError at doc %d: %v\n", docNum, err)
				os.Exit(1)
			}
		}
		elapsed := time.Since(start)

		totalDocs += batchSize
		totalElapsed += elapsed
		throughput := float64(batchSize) / elapsed.Seconds()
		cumAvg := float64(totalDocs) / totalElapsed.Seconds()

		r := batchResult{
			BatchNum:   b + 1,
			DocsTotal:  totalDocs,
			Duration:   elapsed,
			Throughput: throughput,
			CumAvg:     cumAvg,
		}
		results = append(results, r)

		fmt.Printf("  [batch %3d/%d] %4d docs in %8s (%6.0f docs/sec) | total: %5d  avg: %6.0f docs/sec\n",
			b+1, nBatches, batchSize, elapsed.Round(time.Millisecond), throughput, totalDocs, cumAvg)
	}

	fmt.Println()
	fmt.Println("--- Summary ---")
	fmt.Printf("  Total documents: %d\n", totalDocs)
	fmt.Printf("  Total time:      %s\n", totalElapsed.Round(time.Millisecond))
	fmt.Printf("  Avg throughput:  %.0f docs/sec\n", float64(totalDocs)/totalElapsed.Seconds())

	var minT, maxT float64
	minT = math.MaxFloat64
	for _, r := range results {
		if r.Throughput < minT {
			minT = r.Throughput
		}
		if r.Throughput > maxT {
			maxT = r.Throughput
		}
	}
	fmt.Printf("  Min batch:       %.0f docs/sec\n", minT)
	fmt.Printf("  Max batch:       %.0f docs/sec\n", maxT)

	// Generate HTML report
	if err := generateReport(*output, results, *collection, *url, *total, *batch); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating report: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("\n  Report saved to: %s\n", *output)

	// Cleanup
	if *cleanup {
		fmt.Printf("  Cleaning up collection '%s'...\n", *collection)
		if err := deleteCollection(client, *url, *collection); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: cleanup failed: %v\n", err)
		}
	}
}

// --- HTML report ---

type reportData struct {
	Collection string
	URL        string
	Total      int
	Batch      int
	TotalTime  string
	AvgThrpt   float64
	MinThrpt   float64
	MaxThrpt   float64
	Results    []batchResult
	MaxY       float64
	Timestamp  string
}

func generateReport(path string, results []batchResult, collection, url string, total, batch int) error {
	var totalElapsed time.Duration
	var minT, maxT float64
	minT = math.MaxFloat64
	for _, r := range results {
		totalElapsed += r.Duration
		if r.Throughput < minT {
			minT = r.Throughput
		}
		if r.Throughput > maxT {
			maxT = r.Throughput
		}
	}

	maxY := math.Ceil(maxT/100) * 100
	if maxY < 100 {
		maxY = 100
	}

	data := reportData{
		Collection: collection,
		URL:        url,
		Total:      total,
		Batch:      batch,
		TotalTime:  totalElapsed.Round(time.Millisecond).String(),
		AvgThrpt:   float64(total) / totalElapsed.Seconds(),
		MinThrpt:   minT,
		MaxThrpt:   maxT,
		Results:    results,
		MaxY:       maxY,
		Timestamp:  time.Now().Format("2006-01-02 15:04:05"),
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return reportTmpl.Execute(f, data)
}

var reportTmpl = template.Must(template.New("report").Funcs(template.FuncMap{
	"barHeight": func(val, maxY float64) float64 {
		return (val / maxY) * 300
	},
	"barY": func(val, maxY float64) float64 {
		return 300 - (val/maxY)*300
	},
	"lineY": func(val, maxY float64) float64 {
		return 320 - (val/maxY)*300
	},
	"barX": func(idx, total int) float64 {
		if total <= 0 {
			return 0
		}
		w := 800.0 / float64(total)
		return 60 + float64(idx)*w
	},
	"barW": func(total int) float64 {
		if total <= 0 {
			return 5
		}
		w := 800.0 / float64(total)
		if w > 1 {
			w -= 1
		}
		return w
	},
	"fmtDur": func(d time.Duration) string {
		return d.Round(time.Millisecond).String()
	},
	"fmtFloat": func(f float64) string {
		return fmt.Sprintf("%.0f", f)
	},
	"gridY": func(val, maxY float64) float64 {
		return 320 - (val/maxY)*300
	},
	"gridLines": func(maxY float64) []float64 {
		step := maxY / 5
		lines := make([]float64, 5)
		for i := range lines {
			lines[i] = step * float64(i+1)
		}
		return lines
	},
	"mod": func(a, b int) int { return a % b },
}).Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>MDDB Benchmark Report</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; max-width: 960px; margin: 40px auto; padding: 0 20px; color: #1a1a2e; background: #f8f9fa; }
  h1 { color: #16213e; border-bottom: 2px solid #0f3460; padding-bottom: 8px; }
  .stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 16px; margin: 24px 0; }
  .stat { background: #fff; border-radius: 8px; padding: 16px; box-shadow: 0 1px 3px rgba(0,0,0,0.1); }
  .stat .label { font-size: 12px; text-transform: uppercase; color: #666; letter-spacing: 0.5px; }
  .stat .value { font-size: 28px; font-weight: 700; color: #0f3460; margin-top: 4px; }
  .chart-container { background: #fff; border-radius: 8px; padding: 24px; box-shadow: 0 1px 3px rgba(0,0,0,0.1); margin: 24px 0; }
  svg text { font-family: inherit; }
  .bar { fill: #0f3460; opacity: 0.8; }
  .bar:hover { opacity: 1; }
  .avg-line { stroke: #e94560; stroke-width: 2; stroke-dasharray: 6 3; fill: none; }
  .grid-line { stroke: #e0e0e0; stroke-width: 1; }
  .axis-label { font-size: 11px; fill: #666; }
  table { width: 100%; border-collapse: collapse; margin: 24px 0; background: #fff; border-radius: 8px; overflow: hidden; box-shadow: 0 1px 3px rgba(0,0,0,0.1); }
  th { background: #0f3460; color: #fff; padding: 10px 12px; text-align: right; font-size: 13px; }
  th:first-child { text-align: left; }
  td { padding: 8px 12px; text-align: right; border-bottom: 1px solid #eee; font-size: 13px; font-variant-numeric: tabular-nums; }
  td:first-child { text-align: left; }
  tr:hover td { background: #f0f4ff; }
  .footer { margin-top: 32px; color: #999; font-size: 12px; }
</style>
</head>
<body>
<h1>MDDB Benchmark Report</h1>

<div class="stats">
  <div class="stat"><div class="label">Total Documents</div><div class="value">{{.Total}}</div></div>
  <div class="stat"><div class="label">Total Time</div><div class="value">{{.TotalTime}}</div></div>
  <div class="stat"><div class="label">Avg Throughput</div><div class="value">{{fmtFloat .AvgThrpt}} docs/s</div></div>
  <div class="stat"><div class="label">Min Batch</div><div class="value">{{fmtFloat .MinThrpt}} docs/s</div></div>
  <div class="stat"><div class="label">Max Batch</div><div class="value">{{fmtFloat .MaxThrpt}} docs/s</div></div>
  <div class="stat"><div class="label">Batch Size</div><div class="value">{{.Batch}}</div></div>
</div>

<div class="chart-container">
<h2 style="margin-top:0">Throughput per Batch (docs/sec)</h2>
<svg viewBox="0 0 900 380" width="100%" height="380">
  <!-- grid lines -->
  {{- $maxY := .MaxY}}
  {{- $nResults := len .Results}}
  {{range gridLines $maxY}}
  <line x1="60" y1="{{gridY . $maxY}}" x2="860" y2="{{gridY . $maxY}}" class="grid-line"/>
  <text x="55" y="{{gridY . $maxY}}" text-anchor="end" class="axis-label" dy="4">{{fmtFloat .}}</text>
  {{end}}

  <!-- baseline -->
  <line x1="60" y1="320" x2="860" y2="320" stroke="#333" stroke-width="1"/>

  <!-- bars (throughput per batch) -->
  {{range $i, $r := .Results}}
  <rect class="bar" x="{{barX $i $nResults}}" y="{{barY $r.Throughput $maxY}}" width="{{barW $nResults}}" height="{{barHeight $r.Throughput $maxY}}">
    <title>Batch {{$r.BatchNum}}: {{$r.DocsTotal}} docs — {{fmtFloat $r.Throughput}} docs/sec ({{fmtDur $r.Duration}})</title>
  </rect>
  {{end}}

  <!-- cumulative average line -->
  <polyline class="avg-line" points="{{range $i, $r := .Results}}{{barX $i $nResults}},{{lineY $r.CumAvg $maxY}} {{end}}"/>

  <!-- x-axis labels (every 10 batches) -->
  {{range $i, $r := .Results}}
  {{if eq (mod $r.BatchNum 10) 0}}
  <text x="{{barX $i $nResults}}" y="340" text-anchor="middle" class="axis-label">{{$r.DocsTotal}}</text>
  {{end}}
  {{end}}

  <!-- legend -->
  <rect x="660" y="5" width="12" height="12" fill="#0f3460" opacity="0.8"/>
  <text x="678" y="15" class="axis-label">Batch throughput</text>
  <line x1="660" y1="28" x2="672" y2="28" stroke="#e94560" stroke-width="2" stroke-dasharray="6 3"/>
  <text x="678" y="32" class="axis-label">Cumulative average</text>

  <!-- y-axis label -->
  <text x="15" y="170" transform="rotate(-90, 15, 170)" class="axis-label" text-anchor="middle">docs/sec</text>
</svg>
</div>

<h2>Batch Details</h2>
<table>
  <tr><th>Batch</th><th>Docs Total</th><th>Duration</th><th>Throughput</th><th>Cum. Average</th></tr>
  {{range .Results}}
  <tr>
    <td>{{.BatchNum}}</td>
    <td>{{.DocsTotal}}</td>
    <td>{{fmtDur .Duration}}</td>
    <td>{{fmtFloat .Throughput}} docs/sec</td>
    <td>{{fmtFloat .CumAvg}} docs/sec</td>
  </tr>
  {{end}}
</table>

<div class="footer">
  <p>Collection: {{.Collection}} | Server: {{.URL}} | Generated: {{.Timestamp}}</p>
</div>
</body>
</html>
`))

