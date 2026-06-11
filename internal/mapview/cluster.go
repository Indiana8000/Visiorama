// Package mapview provides grid-based GPS clustering for map display.
package mapview

import "math"

// BBox represents a geographic bounding box (West/South/East/North in degrees).
type BBox struct {
	West  float64
	South float64
	East  float64
	North float64
}

// GPSPoint is a single media item with GPS coordinates.
type GPSPoint struct {
	ID  int64
	Lat float64
	Lon float64
}

// Cluster is a group of nearby GPS points collapsed into one map marker.
type Cluster struct {
	Lat         float64 // centroid latitude
	Lon         float64 // centroid longitude
	Count       int     // number of points in this cluster
	IDs         []int64 // all media IDs in this cluster
	ThumbnailID int64   // ID to use for the preview thumbnail (first added)
}

// cellSize returns the geographic grid cell size in degrees for a given zoom level.
// At lower zoom levels the cells are larger, causing more aggressive clustering.
// At zoom >= 15 returns 0, disabling clustering (each point is its own marker).
func cellSize(zoom int) float64 {
	if zoom >= 12 {
		return 0
	}
	// Each zoom step halves the cell size starting from 180° at zoom 0.
	return 180.0 / math.Pow(2, float64(zoom))
}

// GridCluster groups points that fall into the same grid cell at the given zoom level.
// Points outside bbox are excluded before clustering.
func GridCluster(points []GPSPoint, zoom int, bbox BBox) []Cluster {
	if len(points) == 0 {
		return nil
	}

	size := cellSize(zoom)

	type key struct{ col, row int }
	cells := make(map[key]*Cluster)

	for _, p := range points {
		// BBox filter
		if p.Lon < bbox.West || p.Lon > bbox.East ||
			p.Lat < bbox.South || p.Lat > bbox.North {
			continue
		}

		var col, row int
		if size == 0 {
			// No clustering at max zoom: each media ID gets its own marker
			col = int(p.ID)
			row = 0
		} else {
			col = int(math.Floor(p.Lon / size))
			row = int(math.Floor(p.Lat / size))
		}
		k := key{col, row}

		if c, ok := cells[k]; ok {
			c.IDs = append(c.IDs, p.ID)
			c.Lat += p.Lat
			c.Lon += p.Lon
			c.Count++
		} else {
			cells[k] = &Cluster{
				Lat:         p.Lat,
				Lon:         p.Lon,
				Count:       1,
				IDs:         []int64{p.ID},
				ThumbnailID: p.ID,
			}
		}
	}

	result := make([]Cluster, 0, len(cells))
	for _, c := range cells {
		// Finalise centroid as average
		c.Lat /= float64(c.Count)
		c.Lon /= float64(c.Count)
		result = append(result, *c)
	}
	return result
}
