package main

import (
	"context"
	"encoding/json"
	"fmt"
)

func (s *MCPToolServer) toolGeoSearch(ctx context.Context, args map[string]interface{}) (string, error) {
	req := &MCPGeoSearchRequest{
		Collection:   mcpGetString(args, "collection"),
		RadiusMeters: mcpGetFloat(args, "radius_meters"),
		TopK:         mcpGetInt(args, "top_k"),
		Algorithm:    mcpGetString(args, "algorithm"),
		FilterMeta:   mcpGetMetaMap(args, "filter_meta"),
	}
	req.Lat = mcpGetFloat(args, "lat")
	req.Lng = mcpGetFloat(args, "lng")
	if b, ok := args["include_content"].(bool); ok {
		req.IncludeContent = b
	}

	resp, err := s.client.GeoSearch(ctx, req)
	if err != nil {
		return "", err
	}
	data, _ := json.MarshalIndent(resp, "", "  ")
	return string(data), nil
}

func (s *MCPToolServer) toolGeoWithin(ctx context.Context, args map[string]interface{}) (string, error) {
	req := &MCPGeoWithinRequest{
		Collection: mcpGetString(args, "collection"),
		MinLat:     mcpGetFloat(args, "min_lat"),
		MaxLat:     mcpGetFloat(args, "max_lat"),
		MinLng:     mcpGetFloat(args, "min_lng"),
		MaxLng:     mcpGetFloat(args, "max_lng"),
		FilterMeta: mcpGetMetaMap(args, "filter_meta"),
	}
	if b, ok := args["include_content"].(bool); ok {
		req.IncludeContent = b
	}
	resp, err := s.client.GeoWithin(ctx, req)
	if err != nil {
		return "", err
	}
	data, _ := json.MarshalIndent(resp, "", "  ")
	return string(data), nil
}

func (s *MCPToolServer) toolGeoStats(ctx context.Context, args map[string]interface{}) (string, error) {
	_ = args
	resp, err := s.client.GeoStats(ctx)
	if err != nil {
		return "", err
	}
	data, _ := json.MarshalIndent(resp, "", "  ")
	return string(data), nil
}

func (s *MCPToolServer) toolGeoEncode(ctx context.Context, args map[string]interface{}) (string, error) {
	lat := mcpGetFloat(args, "lat")
	lng := mcpGetFloat(args, "lng")
	precision := mcpGetInt(args, "precision")
	h, err := s.client.GeoEncode(ctx, lat, lng, precision)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`{"geohash":%q,"precision":%d}`, h, len(h)), nil
}

func (s *MCPToolServer) toolGeoDecode(ctx context.Context, args map[string]interface{}) (string, error) {
	hash := mcpGetString(args, "geohash")
	lat, lng, err := s.client.GeoDecode(ctx, hash)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`{"lat":%v,"lng":%v}`, lat, lng), nil
}
