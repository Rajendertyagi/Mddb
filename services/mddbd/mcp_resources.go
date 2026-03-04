package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// readResource reads resource based on URI.
func (s *MCPToolServer) readResource(ctx context.Context, uri string) (string, error) {
	parsed, err := url.Parse(uri)
	if err != nil {
		return "", fmt.Errorf("invalid uri: %w", err)
	}

	switch parsed.Scheme {
	case "mddb":
		return s.readMDDBResource(ctx, parsed)
	case "mddb-search":
		return s.readSearchResource(ctx, parsed)
	default:
		return "", fmt.Errorf("unsupported uri scheme: %s", parsed.Scheme)
	}
}

func (s *MCPToolServer) readMDDBResource(ctx context.Context, uri *url.URL) (string, error) {
	path := strings.Trim(uri.Path, "/")

	if path == "health" {
		health, err := s.client.Health(ctx)
		if err != nil {
			return "", err
		}
		data, _ := json.Marshal(health)
		return string(data), nil
	}

	if path == "stats" {
		stats, err := s.client.Stats(ctx)
		if err != nil {
			return "", err
		}
		data, _ := json.Marshal(stats)
		return string(data), nil
	}

	parts := strings.Split(path, "/")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid document uri: expected mddb://{collection}/{key}")
	}

	collection := parts[0]
	key := parts[1]
	lang := uri.Query().Get("lang")
	if lang == "" {
		lang = "en_US"
	}

	envVars := make(map[string]string)
	for k, v := range uri.Query() {
		if strings.HasPrefix(k, "env.") {
			envKey := strings.TrimPrefix(k, "env.")
			if len(v) > 0 {
				envVars[envKey] = v[0]
			}
		}
	}

	doc, err := s.client.Get(ctx, &MCPGetRequest{
		Collection: collection,
		Key:        key,
		Lang:       lang,
		Env:        envVars,
	})
	if err != nil {
		return "", err
	}

	return doc.ContentMD, nil
}

func (s *MCPToolServer) readSearchResource(ctx context.Context, uri *url.URL) (string, error) {
	collection := strings.Trim(uri.Path, "/")
	if collection == "" {
		return "", fmt.Errorf("collection required in search uri")
	}

	query := uri.Query()
	req := &MCPSearchRequest{
		Collection: collection,
		FilterMeta: make(map[string][]string),
	}

	for k, v := range query {
		if strings.HasPrefix(k, "meta.") {
			metaKey := strings.TrimPrefix(k, "meta.")
			req.FilterMeta[metaKey] = v
		}
	}

	if sortVal := query.Get("sort"); sortVal != "" {
		req.Sort = sortVal
	}
	if limit := query.Get("limit"); limit != "" {
		if _, err := fmt.Sscanf(limit, "%d", &req.Limit); err != nil {
			req.Limit = 0
		}
	}
	if offset := query.Get("offset"); offset != "" {
		if _, err := fmt.Sscanf(offset, "%d", &req.Offset); err != nil {
			req.Offset = 0
		}
	}

	resp, err := s.client.Search(ctx, req)
	if err != nil {
		return "", err
	}

	data, _ := json.Marshal(resp)
	return string(data), nil
}
