// Package chinaboundary provides functionality to determine if a latitude and longitude are within mainland China
package chinaboundary

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
)

var (
	geoData     *GeoJSON
	geoDataOnce sync.Once
	geoDataErr  error
)

// GeoJSON defines the structure for GeoJSON data
type GeoJSON struct {
	Type     string    `json:"type"`
	Features []Feature `json:"features"`
}

type Feature struct {
	Type       string     `json:"type"`
	Properties Properties `json:"properties"`
	Geometry   Geometry   `json:"geometry"`
}

type Properties struct {
	Name string `json:"name"`
}

type Geometry struct {
	Type        string          `json:"type"`
	Coordinates json.RawMessage `json:"coordinates"`
}

// Point represents a geographic point with longitude and latitude
type Point struct {
	Lng float64
	Lat float64
}

// IsCN checks if the given latitude and longitude are within mainland China (excluding Hong Kong, Macau, and Taiwan)
// Parameters: latitude, longitude
// Returns: true if the point is in mainland China, false otherwise
func IsCN(latitude, longitude float64) bool {
	// Use sync.Once to ensure data is loaded only once
	geoDataOnce.Do(func() {
		geoData, geoDataErr = loadGeoJSON()
	})

	if geoDataErr != nil {
		fmt.Printf("Failed to load map data: %v\n", geoDataErr)
		return false
	}

	point := Point{Lng: longitude, Lat: latitude}

	// Iterate through all provincial polygons
	for _, feature := range geoData.Features {
		if feature.Geometry.Type == "Polygon" {
			var coords [][][]float64
			if err := json.Unmarshal(feature.Geometry.Coordinates, &coords); err != nil {
				continue
			}
			if isPointInPolygon(point, coords) {
				return true
			}
		} else if feature.Geometry.Type == "MultiPolygon" {
			var coords [][][][]float64
			if err := json.Unmarshal(feature.Geometry.Coordinates, &coords); err != nil {
				continue
			}
			if isPointInMultiPolygon(point, coords) {
				return true
			}
		}
	}

	return false
}

// loadGeoJSON loads China map GeoJSON data from a local file or from CDN
func loadGeoJSON() (*GeoJSON, error) {
	// Provinces to be excluded: Hong Kong, Macau, Taiwan
	excludeProvinces := map[string]bool{
		"香港特别行政区": true,
		"香港":      true,
		"澳门特别行政区": true,
		"澳门":      true,
		"台湾省":     true,
		"台湾":      true,
	}

	var body []byte
	var err error

	// Check if the local file exists
	localFile := "100000_full.json"
	body, err = os.ReadFile(localFile)
	if err != nil {
		// Local file does not exist, download from CDN
		url := "https://geo.datav.aliyun.com/areas_v3/bound/100000_full.json"

		resp, err := http.Get(url)
		if err != nil {
			return nil, fmt.Errorf("download failed: %v", err)
		}
		defer resp.Body.Close()

		body, err = io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %v", err)
		}

		// Save to local file
		if err := os.WriteFile(localFile, body, 0644); err != nil {
			fmt.Printf("warning: failed to save to local file: %v\n", err)
		}
	}

	var geoJSON GeoJSON
	if err := json.Unmarshal(body, &geoJSON); err != nil {
		return nil, err
	}

	// Filter out Hong Kong, Macau, and Taiwan
	filteredFeatures := []Feature{}
	for _, feature := range geoJSON.Features {
		if !excludeProvinces[feature.Properties.Name] {
			filteredFeatures = append(filteredFeatures, feature)
		}
	}
	geoJSON.Features = filteredFeatures

	return &geoJSON, nil
}

// isPointInPolygon checks if a point is inside a polygon
func isPointInPolygon(point Point, polygon [][][]float64) bool {
	for _, ring := range polygon {
		if raycastingAlgorithm(point, ring) {
			return true
		}
	}
	return false
}

// isPointInMultiPolygon checks if a point is inside any of multiple polygons
func isPointInMultiPolygon(point Point, multiPolygon [][][][]float64) bool {
	for _, polygon := range multiPolygon {
		if isPointInPolygon(point, polygon) {
			return true
		}
	}
	return false
}

// raycastingAlgorithm uses ray casting algorithm to determine if a point is inside a polygon
func raycastingAlgorithm(point Point, polygon [][]float64) bool {
	x, y := point.Lng, point.Lat
	inside := false

	for i, j := 0, len(polygon)-1; i < len(polygon); j, i = i, i+1 {
		xi, yi := polygon[i][0], polygon[i][1]
		xj, yj := polygon[j][0], polygon[j][1]

		intersect := ((yi > y) != (yj > y)) &&
			(x < (xj-xi)*(y-yi)/(yj-yi)+xi)

		if intersect {
			inside = !inside
		}
	}

	return inside
}
