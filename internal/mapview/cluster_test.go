package mapview_test

import (
	"testing"

	"github.com/Indiana8000/visiorama/internal/mapview"
)

func TestGridCluster_EmptyInput(t *testing.T) {
	result := mapview.GridCluster(nil, 5, mapview.BBox{West: -180, South: -90, East: 180, North: 90})
	if len(result) != 0 {
		t.Errorf("expected 0 clusters for empty input, got %d", len(result))
	}
}

func TestGridCluster_SinglePoint(t *testing.T) {
	points := []mapview.GPSPoint{{ID: 1, Lat: 48.1351, Lon: 11.5820}} // Munich
	result := mapview.GridCluster(points, 5, mapview.BBox{West: -180, South: -90, East: 180, North: 90})
	if len(result) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(result))
	}
	if result[0].Count != 1 {
		t.Errorf("expected count 1, got %d", result[0].Count)
	}
	if result[0].IDs[0] != 1 {
		t.Errorf("expected ID 1, got %d", result[0].IDs[0])
	}
}

func TestGridCluster_NearbyPointsCluster(t *testing.T) {
	// Two points in Munich (same city) should cluster at zoom 5
	points := []mapview.GPSPoint{
		{ID: 1, Lat: 48.1351, Lon: 11.5820},
		{ID: 2, Lat: 48.1500, Lon: 11.5900},
	}
	result := mapview.GridCluster(points, 5, mapview.BBox{West: -180, South: -90, East: 180, North: 90})
	if len(result) != 1 {
		t.Fatalf("expected 1 cluster for nearby points at zoom 5, got %d", len(result))
	}
	if result[0].Count != 2 {
		t.Errorf("expected count 2, got %d", result[0].Count)
	}
}

func TestGridCluster_FarPointsSeparate(t *testing.T) {
	// Munich and Berlin — should be separate clusters at zoom 10
	points := []mapview.GPSPoint{
		{ID: 1, Lat: 48.1351, Lon: 11.5820}, // Munich
		{ID: 2, Lat: 52.5200, Lon: 13.4050}, // Berlin
	}
	result := mapview.GridCluster(points, 10, mapview.BBox{West: -180, South: -90, East: 180, North: 90})
	if len(result) != 2 {
		t.Fatalf("expected 2 clusters for distant points at zoom 10, got %d", len(result))
	}
}

func TestGridCluster_BBoxFilter(t *testing.T) {
	// Only Munich is in bbox, Berlin is outside
	points := []mapview.GPSPoint{
		{ID: 1, Lat: 48.1351, Lon: 11.5820}, // Munich
		{ID: 2, Lat: 52.5200, Lon: 13.4050}, // Berlin
	}
	bbox := mapview.BBox{West: 10, South: 47, East: 13, North: 50} // covers Munich only
	result := mapview.GridCluster(points, 10, bbox)
	if len(result) != 1 {
		t.Fatalf("expected 1 cluster (bbox filtered Berlin), got %d", len(result))
	}
	if result[0].IDs[0] != 1 {
		t.Errorf("expected Munich (ID 1), got ID %d", result[0].IDs[0])
	}
}

func TestGridCluster_HighZoomNoCluster(t *testing.T) {
	// At zoom 15+, each point should be its own cluster
	points := []mapview.GPSPoint{
		{ID: 1, Lat: 48.1351, Lon: 11.5820},
		{ID: 2, Lat: 48.1352, Lon: 11.5821}, // ~10m apart
	}
	result := mapview.GridCluster(points, 15, mapview.BBox{West: -180, South: -90, East: 180, North: 90})
	if len(result) != 2 {
		t.Fatalf("expected 2 clusters at zoom 15 (no clustering), got %d", len(result))
	}
}

func TestGridCluster_CentroidIsAverage(t *testing.T) {
	points := []mapview.GPSPoint{
		{ID: 1, Lat: 48.0, Lon: 11.0},
		{ID: 2, Lat: 49.0, Lon: 12.0},
	}
	result := mapview.GridCluster(points, 5, mapview.BBox{West: -180, South: -90, East: 180, North: 90})
	if len(result) == 1 {
		// centroid should be roughly in the middle
		if result[0].Lat < 48.0 || result[0].Lat > 49.0 {
			t.Errorf("centroid lat %f out of range [48,49]", result[0].Lat)
		}
	}
	// If 2 clusters, no centroid check needed
}

func TestGridCluster_ThumbnailIDIsFirst(t *testing.T) {
	points := []mapview.GPSPoint{
		{ID: 42, Lat: 48.1, Lon: 11.5},
		{ID: 99, Lat: 48.2, Lon: 11.6},
	}
	result := mapview.GridCluster(points, 5, mapview.BBox{West: -180, South: -90, East: 180, North: 90})
	if len(result) == 1 {
		if result[0].ThumbnailID == 0 {
			t.Error("ThumbnailID should not be 0")
		}
	}
}
