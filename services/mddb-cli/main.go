package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	serverURL  string
	outputJSON bool
	verbose    bool
	apiKey     string // API key for authentication
	token      string // JWT token for authentication
)

type Client struct {
	BaseURL string
	Client  *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
		Client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) request(method, path string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(data)
		if verbose {
			fmt.Fprintf(os.Stderr, "Request: %s\n", string(data))
		}
	}

	req, err := http.NewRequest(method, c.BaseURL+path, reqBody)
	if err != nil {
		return nil, err
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Add authentication headers
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	} else if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("server error (%d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

func main() {
	rootCmd := &cobra.Command{
		Use:   "mddb-cli",
		Short: "MDDB command-line client",
		Long: `mddb-cli is a command-line client for MDDB (Markdown Database).
It provides an interface similar to mysql-client for managing markdown documents.`,
		Version: "1.0.0",
	}

	rootCmd.PersistentFlags().StringVarP(&serverURL, "server", "s", "http://localhost:11023", "MDDB server URL")
	rootCmd.PersistentFlags().BoolVarP(&outputJSON, "json", "j", false, "Output raw JSON")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output")
	rootCmd.PersistentFlags().StringVar(&apiKey, "api-key", "", "API key for authentication")
	rootCmd.PersistentFlags().StringVar(&token, "token", "", "JWT token for authentication")

	// Add command
	addCmd := &cobra.Command{
		Use:   "add [collection] [key] [lang]",
		Short: "Add or update a document",
		Long: `Add or update a markdown document in the database.
Reads content from stdin or file.`,
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			collection, key, lang := args[0], args[1], args[2]

			contentFile, _ := cmd.Flags().GetString("file")
			metaStr, _ := cmd.Flags().GetString("meta")

			var content string
			if contentFile != "" {
				data, err := os.ReadFile(contentFile)
				if err != nil {
					return err
				}
				content = string(data)
			} else {
				data, err := io.ReadAll(os.Stdin)
				if err != nil {
					return err
				}
				content = string(data)
			}

			meta := make(map[string][]string)
			if metaStr != "" {
				pairs := strings.Split(metaStr, ",")
				for _, pair := range pairs {
					kv := strings.SplitN(pair, "=", 2)
					if len(kv) == 2 {
						meta[kv[0]] = strings.Split(kv[1], "|")
					}
				}
			}

			client := NewClient(serverURL)
			body := map[string]interface{}{
				"collection": collection,
				"key":        key,
				"lang":       lang,
				"meta":       meta,
				"contentMd":  content,
			}

			resp, err := client.request("POST", "/v1/add", body)
			if err != nil {
				return err
			}

			if outputJSON {
				fmt.Println(string(resp))
			} else {
				var doc map[string]interface{}
				if err := json.Unmarshal(resp, &doc); err != nil {
					return fmt.Errorf("parse response: %w", err)
				}
				fmt.Printf("✓ Document added: %s\n", doc["id"])
				fmt.Printf("  Added: %v\n", time.Unix(int64(doc["addedAt"].(float64)), 0).Format(time.RFC3339))
				fmt.Printf("  Updated: %v\n", time.Unix(int64(doc["updatedAt"].(float64)), 0).Format(time.RFC3339))
			}

			return nil
		},
	}
	addCmd.Flags().StringP("file", "f", "", "Read content from file instead of stdin")
	addCmd.Flags().StringP("meta", "m", "", "Metadata in format: key=val1|val2,key2=val")

	// Get command
	getCmd := &cobra.Command{
		Use:   "get [collection] [key] [lang]",
		Short: "Get a document",
		Long:  `Retrieve a document from the database.`,
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			collection, key, lang := args[0], args[1], args[2]
			envStr, _ := cmd.Flags().GetString("env")
			contentOnly, _ := cmd.Flags().GetBool("content-only")

			env := make(map[string]string)
			if envStr != "" {
				pairs := strings.Split(envStr, ",")
				for _, pair := range pairs {
					kv := strings.SplitN(pair, "=", 2)
					if len(kv) == 2 {
						env[kv[0]] = kv[1]
					}
				}
			}

			client := NewClient(serverURL)
			body := map[string]interface{}{
				"collection": collection,
				"key":        key,
				"lang":       lang,
				"env":        env,
			}

			resp, err := client.request("POST", "/v1/get", body)
			if err != nil {
				return err
			}

			if contentOnly {
				var doc map[string]interface{}
				if err := json.Unmarshal(resp, &doc); err != nil {
					return fmt.Errorf("parse response: %w", err)
				}
				fmt.Print(doc["contentMd"])
			} else if outputJSON {
				fmt.Println(string(resp))
			} else {
				var doc map[string]interface{}
				if err := json.Unmarshal(resp, &doc); err != nil {
					return fmt.Errorf("parse response: %w", err)
				}
				fmt.Printf("ID: %s\n", doc["id"])
				fmt.Printf("Key: %s\n", doc["key"])
				fmt.Printf("Lang: %s\n", doc["lang"])
				fmt.Printf("Added: %v\n", time.Unix(int64(doc["addedAt"].(float64)), 0).Format(time.RFC3339))
				fmt.Printf("Updated: %v\n", time.Unix(int64(doc["updatedAt"].(float64)), 0).Format(time.RFC3339))
				if meta, ok := doc["meta"].(map[string]interface{}); ok && len(meta) > 0 {
					fmt.Println("Meta:")
					for k, v := range meta {
						fmt.Printf("  %s: %v\n", k, v)
					}
				}
				fmt.Println("\nContent:")
				fmt.Println(strings.Repeat("-", 80))
				fmt.Println(doc["contentMd"])
			}

			return nil
		},
	}
	getCmd.Flags().StringP("env", "e", "", "Environment variables for templating: key=val,key2=val2")
	getCmd.Flags().BoolP("content-only", "c", false, "Output only content (no metadata)")

	// Search command
	searchCmd := &cobra.Command{
		Use:   "search [collection]",
		Short: "Search documents",
		Long:  `Search documents in a collection with optional filters.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			collection := args[0]
			metaStr, _ := cmd.Flags().GetString("filter")
			sort, _ := cmd.Flags().GetString("sort")
			asc, _ := cmd.Flags().GetBool("asc")
			limit, _ := cmd.Flags().GetInt("limit")
			offset, _ := cmd.Flags().GetInt("offset")

			filterMeta := make(map[string][]string)
			if metaStr != "" {
				pairs := strings.Split(metaStr, ",")
				for _, pair := range pairs {
					kv := strings.SplitN(pair, "=", 2)
					if len(kv) == 2 {
						filterMeta[kv[0]] = strings.Split(kv[1], "|")
					}
				}
			}

			client := NewClient(serverURL)
			body := map[string]interface{}{
				"collection": collection,
				"filterMeta": filterMeta,
				"sort":       sort,
				"asc":        asc,
				"limit":      limit,
				"offset":     offset,
			}

			resp, err := client.request("POST", "/v1/search", body)
			if err != nil {
				return err
			}

			if outputJSON {
				fmt.Println(string(resp))
			} else {
				var docs []map[string]interface{}
				json.Unmarshal(resp, &docs)
				fmt.Printf("Found %d documents:\n\n", len(docs))
				for i, doc := range docs {
					fmt.Printf("%d. %s (%s)\n", i+1, doc["key"], doc["lang"])
					fmt.Printf("   ID: %s\n", doc["id"])
					fmt.Printf("   Updated: %v\n", time.Unix(int64(doc["updatedAt"].(float64)), 0).Format(time.RFC3339))
					if meta, ok := doc["meta"].(map[string]interface{}); ok && len(meta) > 0 {
						fmt.Print("   Meta: ")
						metaParts := []string{}
						for k, v := range meta {
							metaParts = append(metaParts, fmt.Sprintf("%s=%v", k, v))
						}
						fmt.Println(strings.Join(metaParts, ", "))
					}
					fmt.Println()
				}
			}

			return nil
		},
	}
	searchCmd.Flags().StringP("filter", "f", "", "Filter by metadata: key=val1|val2,key2=val")
	searchCmd.Flags().StringP("sort", "S", "updatedAt", "Sort field: addedAt, updatedAt, key")
	searchCmd.Flags().BoolP("asc", "a", false, "Sort ascending (default: descending)")
	searchCmd.Flags().IntP("limit", "l", 50, "Limit results")
	searchCmd.Flags().IntP("offset", "o", 0, "Offset results")

	// Export command
	exportCmd := &cobra.Command{
		Use:   "export [collection]",
		Short: "Export documents",
		Long:  `Export documents from a collection as NDJSON or ZIP.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			collection := args[0]
			format, _ := cmd.Flags().GetString("format")
			output, _ := cmd.Flags().GetString("output")
			metaStr, _ := cmd.Flags().GetString("filter")

			filterMeta := make(map[string][]string)
			if metaStr != "" {
				pairs := strings.Split(metaStr, ",")
				for _, pair := range pairs {
					kv := strings.SplitN(pair, "=", 2)
					if len(kv) == 2 {
						filterMeta[kv[0]] = strings.Split(kv[1], "|")
					}
				}
			}

			client := NewClient(serverURL)
			body := map[string]interface{}{
				"collection": collection,
				"filterMeta": filterMeta,
				"format":     format,
			}

			resp, err := client.request("POST", "/v1/export", body)
			if err != nil {
				return err
			}

			if output != "" {
				err = os.WriteFile(output, resp, 0644)
				if err != nil {
					return err
				}
				fmt.Printf("✓ Exported to: %s\n", output)
			} else {
				fmt.Print(string(resp))
			}

			return nil
		},
	}
	exportCmd.Flags().StringP("format", "F", "ndjson", "Export format: ndjson, zip")
	exportCmd.Flags().StringP("output", "o", "", "Output file (default: stdout)")
	exportCmd.Flags().StringP("filter", "f", "", "Filter by metadata: key=val1|val2,key2=val")

	// Backup command
	backupCmd := &cobra.Command{
		Use:   "backup [filename]",
		Short: "Create database backup",
		Long:  `Create a backup of the database.`,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filename := fmt.Sprintf("backup-%d.db", time.Now().Unix())
			if len(args) > 0 {
				filename = args[0]
			}

			client := NewClient(serverURL)
			resp, err := client.request("GET", fmt.Sprintf("/v1/backup?to=%s", filename), nil)
			if err != nil {
				return err
			}

			if outputJSON {
				fmt.Println(string(resp))
			} else {
				var result map[string]string
				json.Unmarshal(resp, &result)
				fmt.Printf("✓ Backup created: %s\n", result["backup"])
			}

			return nil
		},
	}

	// Restore command
	restoreCmd := &cobra.Command{
		Use:   "restore [filename]",
		Short: "Restore database from backup",
		Long:  `Restore the database from a backup file.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filename := args[0]

			client := NewClient(serverURL)
			body := map[string]string{"from": filename}
			resp, err := client.request("POST", "/v1/restore", body)
			if err != nil {
				return err
			}

			if outputJSON {
				fmt.Println(string(resp))
			} else {
				var result map[string]string
				json.Unmarshal(resp, &result)
				fmt.Printf("✓ Restored from: %s\n", result["restored"])
			}

			return nil
		},
	}

	// Truncate command
	truncateCmd := &cobra.Command{
		Use:   "truncate [collection]",
		Short: "Truncate revision history",
		Long:  `Remove old revisions from a collection.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			collection := args[0]
			keepRevs, _ := cmd.Flags().GetInt("keep")
			dropCache, _ := cmd.Flags().GetBool("drop-cache")

			client := NewClient(serverURL)
			body := map[string]interface{}{
				"collection": collection,
				"keepRevs":   keepRevs,
				"dropCache":  dropCache,
			}

			resp, err := client.request("POST", "/v1/truncate", body)
			if err != nil {
				return err
			}

			if outputJSON {
				fmt.Println(string(resp))
			} else {
				fmt.Printf("✓ Truncated revisions in collection: %s\n", collection)
				fmt.Printf("  Kept last %d revisions per document\n", keepRevs)
			}

			return nil
		},
	}
	truncateCmd.Flags().IntP("keep", "k", 5, "Number of revisions to keep")
	truncateCmd.Flags().BoolP("drop-cache", "d", true, "Drop cache")

	// Stats command
	statsCmd := &cobra.Command{
		Use:   "stats",
		Short: "Show server statistics",
		Long:  `Display database statistics including collection counts, revisions, and size.`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := NewClient(serverURL)
			resp, err := client.request("GET", "/v1/stats", nil)
			if err != nil {
				return err
			}

			if outputJSON {
				fmt.Println(string(resp))
			} else {
				var stats map[string]interface{}
				json.Unmarshal(resp, &stats)

				fmt.Printf("MDDB Server Statistics\n")
				fmt.Printf("═══════════════════════════════════════\n\n")
				fmt.Printf("Database Path: %s\n", stats["databasePath"])
				fmt.Printf("Database Size: %.2f MB\n", float64(stats["databaseSize"].(float64))/1024/1024)
				fmt.Printf("Access Mode:   %s\n\n", stats["mode"])

				fmt.Printf("Global Totals:\n")
				fmt.Printf("  Documents:     %d\n", int(stats["totalDocuments"].(float64)))
				fmt.Printf("  Revisions:     %d\n", int(stats["totalRevisions"].(float64)))
				fmt.Printf("  Meta Indices:  %d\n\n", int(stats["totalMetaIndices"].(float64)))

				if collections, ok := stats["collections"].([]interface{}); ok && len(collections) > 0 {
					fmt.Printf("Collections:\n")
					fmt.Printf("─────────────────────────────────────────\n")
					fmt.Printf("%-20s %10s %10s %10s\n", "Name", "Docs", "Revs", "Indices")
					fmt.Printf("─────────────────────────────────────────\n")
					for _, c := range collections {
						coll := c.(map[string]interface{})
						fmt.Printf("%-20s %10d %10d %10d\n",
							coll["name"],
							int(coll["documentCount"].(float64)),
							int(coll["revisionCount"].(float64)),
							int(coll["metaIndexCount"].(float64)))
					}
				} else {
					fmt.Printf("No collections found.\n")
				}
			}

			return nil
		},
	}

	// Vector Search command
	vectorSearchCmd := &cobra.Command{
		Use:   "vector-search [collection]",
		Short: "Semantic search using AI embeddings",
		Long:  `Search documents by meaning using vector similarity. Requires embedding provider configured on server.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			collection := args[0]
			query, _ := cmd.Flags().GetString("query")
			topK, _ := cmd.Flags().GetInt("top-k")
			threshold, _ := cmd.Flags().GetFloat64("threshold")
			metaStr, _ := cmd.Flags().GetString("filter")
			includeContent, _ := cmd.Flags().GetBool("include-content")

			if query == "" {
				return fmt.Errorf("--query flag is required")
			}

			filterMeta := make(map[string][]string)
			if metaStr != "" {
				pairs := strings.Split(metaStr, ",")
				for _, pair := range pairs {
					kv := strings.SplitN(pair, "=", 2)
					if len(kv) == 2 {
						filterMeta[kv[0]] = strings.Split(kv[1], "|")
					}
				}
			}

			client := NewClient(serverURL)
			body := map[string]interface{}{
				"collection":     collection,
				"query":          query,
				"topK":           topK,
				"threshold":      threshold,
				"includeContent": includeContent,
			}
			if len(filterMeta) > 0 {
				body["filterMeta"] = filterMeta
			}

			resp, err := client.request("POST", "/v1/vector-search", body)
			if err != nil {
				return err
			}

			if outputJSON {
				fmt.Println(string(resp))
			} else {
				var result map[string]interface{}
				json.Unmarshal(resp, &result)

				results, _ := result["results"].([]interface{})
				model, _ := result["model"].(string)
				dims, _ := result["dimensions"].(float64)

				fmt.Printf("Vector Search Results (model: %s, dims: %d)\n", model, int(dims))
				fmt.Printf("Query: %q\n", query)
				fmt.Printf("═══════════════════════════════════════\n\n")

				if len(results) == 0 {
					fmt.Println("No results found.")
				} else {
					for _, r := range results {
						item := r.(map[string]interface{})
						doc := item["document"].(map[string]interface{})
						score := item["score"].(float64)
						rank := int(item["rank"].(float64))

						fmt.Printf("#%d  %.0f%%  %s (%s)\n", rank, score*100, doc["key"], doc["lang"])
						if meta, ok := doc["meta"].(map[string]interface{}); ok && len(meta) > 0 {
							metaParts := []string{}
							for k, v := range meta {
								metaParts = append(metaParts, fmt.Sprintf("%s=%v", k, v))
							}
							fmt.Printf("     Meta: %s\n", strings.Join(metaParts, ", "))
						}
						if includeContent {
							if content, ok := doc["contentMd"].(string); ok && content != "" {
								preview := content
								if len(preview) > 200 {
									preview = preview[:200] + "..."
								}
								fmt.Printf("     Content: %s\n", strings.ReplaceAll(preview, "\n", " "))
							}
						}
						fmt.Println()
					}
				}
			}

			return nil
		},
	}
	vectorSearchCmd.Flags().StringP("query", "q", "", "Search query (required)")
	vectorSearchCmd.Flags().IntP("top-k", "k", 5, "Number of results")
	vectorSearchCmd.Flags().Float64P("threshold", "t", 0.0, "Minimum similarity score (0.0-1.0)")
	vectorSearchCmd.Flags().StringP("filter", "f", "", "Pre-filter by metadata: key=val1|val2,key2=val")
	vectorSearchCmd.Flags().BoolP("include-content", "c", false, "Include document content in results")

	// Vector Reindex command
	vectorReindexCmd := &cobra.Command{
		Use:   "vector-reindex [collection]",
		Short: "Re-embed documents in a collection",
		Long:  `Re-generate embedding vectors for all documents in a collection.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			collection := args[0]
			force, _ := cmd.Flags().GetBool("force")

			client := NewClient(serverURL)
			body := map[string]interface{}{
				"collection": collection,
				"force":      force,
			}

			resp, err := client.request("POST", "/v1/vector-reindex", body)
			if err != nil {
				return err
			}

			if outputJSON {
				fmt.Println(string(resp))
			} else {
				var result map[string]interface{}
				json.Unmarshal(resp, &result)
				embedded := int(result["embedded"].(float64))
				skipped := int(result["skipped"].(float64))
				failed := int(result["failed"].(float64))

				fmt.Printf("Reindex completed for collection: %s\n", collection)
				fmt.Printf("  Embedded: %d\n", embedded)
				fmt.Printf("  Skipped:  %d\n", skipped)
				fmt.Printf("  Failed:   %d\n", failed)

				if errs, ok := result["errors"].([]interface{}); ok && len(errs) > 0 {
					fmt.Printf("\n  Errors:\n")
					for _, e := range errs {
						fmt.Printf("    - %s\n", e)
					}
				}
			}

			return nil
		},
	}
	vectorReindexCmd.Flags().Bool("force", false, "Force re-embed all documents (ignore content hash)")

	// Vector Stats command
	vectorStatsCmd := &cobra.Command{
		Use:   "vector-stats",
		Short: "Show vector/embedding statistics",
		Long:  `Display embedding provider info and per-collection embedding statistics.`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := NewClient(serverURL)
			resp, err := client.request("GET", "/v1/vector-stats", nil)
			if err != nil {
				return err
			}

			if outputJSON {
				fmt.Println(string(resp))
			} else {
				var result map[string]interface{}
				json.Unmarshal(resp, &result)

				enabled, _ := result["enabled"].(bool)
				fmt.Printf("Vector Search Statistics\n")
				fmt.Printf("═══════════════════════════════════════\n\n")

				if !enabled {
					fmt.Println("Embedding provider: disabled")
					fmt.Println("Set MDDB_EMBEDDING_PROVIDER to enable vector search.")
					return nil
				}

				fmt.Printf("Provider:   %s\n", result["provider"])
				fmt.Printf("Model:      %s\n", result["model"])
				fmt.Printf("Dimensions: %v\n", result["dimensions"])
				fmt.Printf("Index Ready: %v\n\n", result["index_ready"])

				if collections, ok := result["collections"].(map[string]interface{}); ok && len(collections) > 0 {
					fmt.Printf("Collections:\n")
					fmt.Printf("─────────────────────────────────────────\n")
					fmt.Printf("%-20s %12s %12s %10s\n", "Name", "Documents", "Embedded", "Coverage")
					fmt.Printf("─────────────────────────────────────────\n")
					for name, v := range collections {
						coll := v.(map[string]interface{})
						total := int(coll["total_documents"].(float64))
						embedded := int(coll["embedded_documents"].(float64))
						coverage := 0.0
						if total > 0 {
							coverage = float64(embedded) / float64(total) * 100
						}
						fmt.Printf("%-20s %12d %12d %9.0f%%\n", name, total, embedded, coverage)
					}
				} else {
					fmt.Println("No collections with embeddings.")
				}
			}

			return nil
		},
	}

	// Import URL command
	importURLCmd := &cobra.Command{
		Use:   "import-url [collection] [url] [lang]",
		Short: "Import a document from URL",
		Long:  `Import a markdown document from a URL. YAML frontmatter is parsed as metadata.`,
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			collection, url, lang := args[0], args[1], args[2]
			key, _ := cmd.Flags().GetString("key")
			metaStr, _ := cmd.Flags().GetString("meta")
			ttl, _ := cmd.Flags().GetInt64("ttl")

			meta := make(map[string][]string)
			if metaStr != "" {
				pairs := strings.Split(metaStr, ",")
				for _, pair := range pairs {
					kv := strings.SplitN(pair, "=", 2)
					if len(kv) == 2 {
						meta[kv[0]] = strings.Split(kv[1], "|")
					}
				}
			}

			client := NewClient(serverURL)
			body := map[string]interface{}{
				"collection": collection,
				"url":        url,
				"lang":       lang,
			}
			if key != "" {
				body["key"] = key
			}
			if len(meta) > 0 {
				body["meta"] = meta
			}
			if ttl > 0 {
				body["ttl"] = ttl
			}

			resp, err := client.request("POST", "/v1/import-url", body)
			if err != nil {
				return err
			}

			if outputJSON {
				fmt.Println(string(resp))
			} else {
				var doc map[string]interface{}
				json.Unmarshal(resp, &doc)
				fmt.Printf("Document imported from URL: %s\n", doc["id"])
			}
			return nil
		},
	}
	importURLCmd.Flags().String("key", "", "Document key (auto-derived from URL if empty)")
	importURLCmd.Flags().StringP("meta", "m", "", "Metadata: key=val1|val2,key2=val")
	importURLCmd.Flags().Int64("ttl", 0, "TTL in seconds (0 = no expiry)")

	// Set TTL command
	setTTLCmd := &cobra.Command{
		Use:   "set-ttl [collection] [key] [lang]",
		Short: "Set TTL on a document",
		Long:  `Set or remove time-to-live on a document. Use --ttl=0 to remove TTL.`,
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			collection, key, lang := args[0], args[1], args[2]
			ttl, _ := cmd.Flags().GetInt64("ttl")

			client := NewClient(serverURL)
			body := map[string]interface{}{
				"collection": collection,
				"key":        key,
				"lang":       lang,
				"ttl":        ttl,
			}

			resp, err := client.request("POST", "/v1/set-ttl", body)
			if err != nil {
				return err
			}

			if outputJSON {
				fmt.Println(string(resp))
			} else {
				if ttl > 0 {
					fmt.Printf("TTL set to %d seconds for %s/%s/%s\n", ttl, collection, key, lang)
				} else {
					fmt.Printf("TTL removed for %s/%s/%s\n", collection, key, lang)
				}
			}
			return nil
		},
	}
	setTTLCmd.Flags().Int64("ttl", 0, "TTL in seconds (0 = remove)")

	// FTS command
	ftsCmd := &cobra.Command{
		Use:   "fts [collection]",
		Short: "Full-text search",
		Long:  `Search documents by text content using full-text search.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			collection := args[0]
			query, _ := cmd.Flags().GetString("query")
			limit, _ := cmd.Flags().GetInt("limit")

			if query == "" {
				return fmt.Errorf("--query flag is required")
			}

			client := NewClient(serverURL)
			body := map[string]interface{}{
				"collection": collection,
				"query":      query,
				"limit":      limit,
			}

			resp, err := client.request("POST", "/v1/fts", body)
			if err != nil {
				return err
			}

			if outputJSON {
				fmt.Println(string(resp))
			} else {
				var result map[string]interface{}
				json.Unmarshal(resp, &result)

				results, _ := result["results"].([]interface{})
				total, _ := result["total"].(float64)

				fmt.Printf("Full-Text Search Results (%d matches)\n", int(total))
				fmt.Printf("Query: %q\n", query)
				fmt.Printf("═══════════════════════════════════════\n\n")

				if len(results) == 0 {
					fmt.Println("No results found.")
				} else {
					for i, r := range results {
						item := r.(map[string]interface{})
						doc := item["document"].(map[string]interface{})
						score := item["score"].(float64)
						terms, _ := item["matchedTerms"].([]interface{})

						fmt.Printf("%d. %.0f%%  %s (%s)\n", i+1, score*100, doc["key"], doc["lang"])
						if len(terms) > 0 {
							termStrs := make([]string, len(terms))
							for j, t := range terms {
								termStrs[j] = t.(string)
							}
							fmt.Printf("   Matched: %s\n", strings.Join(termStrs, ", "))
						}
						fmt.Println()
					}
				}
			}
			return nil
		},
	}
	ftsCmd.Flags().StringP("query", "q", "", "Search query (required)")
	ftsCmd.Flags().IntP("limit", "l", 50, "Max results")

	// Webhook commands
	webhookCmd := &cobra.Command{
		Use:   "webhook",
		Short: "Manage webhooks",
		Long:  `Register, list, and delete webhook subscriptions.`,
	}

	webhookRegisterCmd := &cobra.Command{
		Use:   "register",
		Short: "Register a webhook",
		RunE: func(cmd *cobra.Command, args []string) error {
			url, _ := cmd.Flags().GetString("url")
			eventsStr, _ := cmd.Flags().GetString("events")
			collection, _ := cmd.Flags().GetString("collection")

			if url == "" || eventsStr == "" {
				return fmt.Errorf("--url and --events flags are required")
			}

			events := strings.Split(eventsStr, ",")

			client := NewClient(serverURL)
			body := map[string]interface{}{
				"url":    url,
				"events": events,
			}
			if collection != "" {
				body["collection"] = collection
			}

			resp, err := client.request("POST", "/v1/webhooks", body)
			if err != nil {
				return err
			}

			if outputJSON {
				fmt.Println(string(resp))
			} else {
				var wh map[string]interface{}
				json.Unmarshal(resp, &wh)
				fmt.Printf("Webhook registered: %s\n", wh["id"])
				fmt.Printf("  URL: %s\n", wh["url"])
				fmt.Printf("  Events: %v\n", wh["events"])
			}
			return nil
		},
	}
	webhookRegisterCmd.Flags().String("url", "", "Webhook endpoint URL (required)")
	webhookRegisterCmd.Flags().String("events", "", "Events: doc.added,doc.updated,doc.deleted (required)")
	webhookRegisterCmd.Flags().String("collection", "", "Filter to collection (optional)")

	webhookListCmd := &cobra.Command{
		Use:   "list",
		Short: "List webhooks",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := NewClient(serverURL)
			resp, err := client.request("GET", "/v1/webhooks", nil)
			if err != nil {
				return err
			}

			if outputJSON {
				fmt.Println(string(resp))
			} else {
				var hooks []map[string]interface{}
				json.Unmarshal(resp, &hooks)
				if len(hooks) == 0 {
					fmt.Println("No webhooks registered.")
				} else {
					fmt.Printf("%-20s %-40s %-30s %s\n", "ID", "URL", "Events", "Collection")
					fmt.Printf("%-20s %-40s %-30s %s\n", "──────────────────", "────────────────────────────────────────", "──────────────────────────────", "──────────")
					for _, h := range hooks {
						coll := ""
						if c, ok := h["collection"].(string); ok {
							coll = c
						}
						fmt.Printf("%-20s %-40s %-30v %s\n", h["id"], h["url"], h["events"], coll)
					}
				}
			}
			return nil
		},
	}

	webhookDeleteCmd := &cobra.Command{
		Use:   "delete [id]",
		Short: "Delete a webhook",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			client := NewClient(serverURL)
			body := map[string]interface{}{"id": id}
			resp, err := client.request("POST", "/v1/webhooks/delete", body)
			if err != nil {
				return err
			}

			if outputJSON {
				fmt.Println(string(resp))
			} else {
				fmt.Printf("Webhook deleted: %s\n", id)
			}
			return nil
		},
	}

	webhookCmd.AddCommand(webhookRegisterCmd, webhookListCmd, webhookDeleteCmd)

	// Schema commands
	schemaCmd := &cobra.Command{
		Use:   "schema",
		Short: "Manage collection schemas",
		Long:  `Set, get, delete, and list JSON schemas for collection validation.`,
	}

	schemaSetCmd := &cobra.Command{
		Use:   "set",
		Short: "Set schema for a collection",
		RunE: func(cmd *cobra.Command, args []string) error {
			collection, _ := cmd.Flags().GetString("collection")
			schema, _ := cmd.Flags().GetString("schema")

			if collection == "" || schema == "" {
				return fmt.Errorf("--collection and --schema flags are required")
			}

			client := NewClient(serverURL)
			body := map[string]interface{}{
				"collection": collection,
				"schema":     schema,
			}

			resp, err := client.request("POST", "/v1/schema/set", body)
			if err != nil {
				return err
			}

			if outputJSON {
				fmt.Println(string(resp))
			} else {
				fmt.Printf("Schema set for collection: %s\n", collection)
			}
			return nil
		},
	}
	schemaSetCmd.Flags().StringP("collection", "c", "", "Collection name (required)")
	schemaSetCmd.Flags().StringP("schema", "s", "", "JSON schema string (required)")

	schemaGetCmd := &cobra.Command{
		Use:   "get",
		Short: "Get schema for a collection",
		RunE: func(cmd *cobra.Command, args []string) error {
			collection, _ := cmd.Flags().GetString("collection")

			if collection == "" {
				return fmt.Errorf("--collection flag is required")
			}

			client := NewClient(serverURL)
			body := map[string]interface{}{
				"collection": collection,
			}

			resp, err := client.request("POST", "/v1/schema/get", body)
			if err != nil {
				return err
			}

			if outputJSON {
				fmt.Println(string(resp))
			} else {
				var result map[string]interface{}
				json.Unmarshal(resp, &result)
				if schema, ok := result["schema"].(string); ok {
					fmt.Printf("Schema for collection %q:\n%s\n", collection, schema)
				} else {
					// Pretty-print the whole response
					pretty, _ := json.MarshalIndent(result, "", "  ")
					fmt.Printf("Schema for collection %q:\n%s\n", collection, string(pretty))
				}
			}
			return nil
		},
	}
	schemaGetCmd.Flags().StringP("collection", "c", "", "Collection name (required)")

	schemaDeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete schema for a collection",
		RunE: func(cmd *cobra.Command, args []string) error {
			collection, _ := cmd.Flags().GetString("collection")

			if collection == "" {
				return fmt.Errorf("--collection flag is required")
			}

			client := NewClient(serverURL)
			body := map[string]interface{}{
				"collection": collection,
			}

			resp, err := client.request("POST", "/v1/schema/delete", body)
			if err != nil {
				return err
			}

			if outputJSON {
				fmt.Println(string(resp))
			} else {
				fmt.Printf("Schema deleted for collection: %s\n", collection)
			}
			return nil
		},
	}
	schemaDeleteCmd.Flags().StringP("collection", "c", "", "Collection name (required)")

	schemaListCmd := &cobra.Command{
		Use:   "list",
		Short: "List all schemas",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := NewClient(serverURL)
			body := map[string]interface{}{}

			resp, err := client.request("POST", "/v1/schema/list", body)
			if err != nil {
				return err
			}

			if outputJSON {
				fmt.Println(string(resp))
			} else {
				var result map[string]interface{}
				json.Unmarshal(resp, &result)

				if schemas, ok := result["schemas"].(map[string]interface{}); ok && len(schemas) > 0 {
					fmt.Printf("Schemas:\n")
					fmt.Printf("─────────────────────────────────────────\n")
					for name, schema := range schemas {
						fmt.Printf("%-20s %v\n", name, schema)
					}
				} else if schemas, ok := result["schemas"].([]interface{}); ok && len(schemas) > 0 {
					fmt.Printf("Schemas:\n")
					fmt.Printf("─────────────────────────────────────────\n")
					for _, s := range schemas {
						item := s.(map[string]interface{})
						fmt.Printf("%-20s %v\n", item["collection"], item["schema"])
					}
				} else {
					fmt.Println("No schemas found.")
				}
			}
			return nil
		},
	}

	schemaCmd.AddCommand(schemaSetCmd, schemaGetCmd, schemaDeleteCmd, schemaListCmd)

	// Validate command
	validateCmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate document metadata against schema",
		Long:  `Validate document metadata against the JSON schema defined for a collection.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			collection, _ := cmd.Flags().GetString("collection")
			metaStr, _ := cmd.Flags().GetString("meta")

			if collection == "" || metaStr == "" {
				return fmt.Errorf("--collection and --meta flags are required")
			}

			var meta interface{}
			if err := json.Unmarshal([]byte(metaStr), &meta); err != nil {
				return fmt.Errorf("invalid JSON for --meta: %w", err)
			}

			client := NewClient(serverURL)
			body := map[string]interface{}{
				"collection": collection,
				"meta":       meta,
			}

			resp, err := client.request("POST", "/v1/validate", body)
			if err != nil {
				return err
			}

			if outputJSON {
				fmt.Println(string(resp))
			} else {
				var result map[string]interface{}
				json.Unmarshal(resp, &result)

				if valid, ok := result["valid"].(bool); ok && valid {
					fmt.Printf("Metadata is valid for collection: %s\n", collection)
				} else {
					fmt.Printf("Validation failed for collection: %s\n", collection)
					if errors, ok := result["errors"].([]interface{}); ok {
						for _, e := range errors {
							fmt.Printf("  - %v\n", e)
						}
					}
					if msg, ok := result["error"].(string); ok {
						fmt.Printf("  Error: %s\n", msg)
					}
				}
			}
			return nil
		},
	}
	validateCmd.Flags().StringP("collection", "c", "", "Collection name (required)")
	validateCmd.Flags().StringP("meta", "m", "", "Metadata as JSON string (required)")

	// Login command
	loginCmd := &cobra.Command{
		Use:   "login [username] [password]",
		Short: "Login and get JWT token",
		Long:  `Authenticate with MDDB server using username and password, receive JWT token.`,
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			username, password := args[0], args[1]

			client := NewClient(serverURL)
			body := map[string]interface{}{
				"username": username,
				"password": password,
			}

			resp, err := client.request("POST", "/v1/auth/login", body)
			if err != nil {
				return fmt.Errorf("login failed: %w", err)
			}

			if outputJSON {
				fmt.Println(string(resp))
			} else {
				var result map[string]interface{}
				json.Unmarshal(resp, &result)

				tokenStr, _ := result["token"].(string)
				expiresAt, _ := result["expiresAt"].(float64)

				fmt.Printf("✓ Login successful\n")
				fmt.Printf("Token: %s\n", tokenStr)
				fmt.Printf("Expires: %v\n", time.Unix(int64(expiresAt), 0).Format(time.RFC3339))
				fmt.Printf("\nUse with: mddb-cli --token %s <command>\n", tokenStr)
			}
			return nil
		},
	}

	rootCmd.AddCommand(addCmd, getCmd, searchCmd, exportCmd, backupCmd, restoreCmd, truncateCmd, statsCmd, vectorSearchCmd, vectorReindexCmd, vectorStatsCmd, importURLCmd, setTTLCmd, ftsCmd, webhookCmd, schemaCmd, validateCmd, loginCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
