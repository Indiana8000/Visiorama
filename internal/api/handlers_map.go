package api

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/Indiana8000/visiorama/internal/index"
	"github.com/Indiana8000/visiorama/internal/index/repositories"
	"github.com/Indiana8000/visiorama/internal/mapview"
)

const openFreeMapStyle = "https://tiles.openfreemap.org/styles/liberty"

type mapHandler struct {
	store     *index.Store
	styleMu   sync.Mutex
	styleBody []byte
	styleAt   time.Time
}

// getStyle proxies the OpenFreeMap style JSON so the browser avoids CORS.
// Cached for 10 minutes.
func (h *mapHandler) getStyle(w http.ResponseWriter, r *http.Request) {
	h.styleMu.Lock()
	if time.Since(h.styleAt) > 10*time.Minute {
		resp, err := http.Get(openFreeMapStyle) //nolint:gosec
		if err != nil {
			h.styleMu.Unlock()
			http.Error(w, "upstream unavailable", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			h.styleMu.Unlock()
			http.Error(w, "upstream read error", http.StatusBadGateway)
			return
		}
		h.styleBody = body
		h.styleAt = time.Now()
	}
	body := h.styleBody
	h.styleMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=600")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// GeoJSONFeatureCollection is returned by /api/map/clusters
type GeoJSONFeatureCollection struct {
	Type     string           `json:"type"`
	Features []GeoJSONFeature `json:"features"`
}

type GeoJSONFeature struct {
	Type       string            `json:"type"`
	Geometry   GeoJSONPoint      `json:"geometry"`
	Properties ClusterProperties `json:"properties"`
}

type GeoJSONPoint struct {
	Type        string     `json:"type"`
	Coordinates [2]float64 `json:"coordinates"` // [lon, lat]
}

type ClusterProperties struct {
	Count       int     `json:"count"`
	IDs         []int64 `json:"ids"`
	ThumbnailID int64   `json:"thumbnailId"`
}

func (h *mapHandler) getClusters(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	// zoom
	zoom := 5
	if v := q.Get("zoom"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 22 {
			badRequest(w, "zoom must be 0-22")
			return
		}
		zoom = n
	}

	// bbox: west,south,east,north
	bbox := mapview.BBox{West: -180, South: -90, East: 180, North: 90}
	if v := q.Get("bbox"); v != "" {
		var west, south, east, north float64
		if _, err := fmt.Sscanf(v, "%f,%f,%f,%f", &west, &south, &east, &north); err != nil {
			badRequest(w, "bbox must be west,south,east,north")
			return
		}
		bbox = mapview.BBox{West: west, South: south, East: east, North: north}
	}

	// optional album_id filter
	var albumID *int64
	if v := q.Get("album_id"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			badRequest(w, "album_id must be an integer")
			return
		}
		albumID = &n
	}

	mediaRepo := repositories.NewMediaRepo(h.store.DB())
	gpsMedia, err := mediaRepo.GetGPSMedia(albumID)
	if err != nil {
		internalError(w)
		return
	}

	points := make([]mapview.GPSPoint, len(gpsMedia))
	for i, m := range gpsMedia {
		points[i] = mapview.GPSPoint{ID: m.ID, Lat: m.GpsLat, Lon: m.GpsLon}
	}

	clusters := mapview.GridCluster(points, zoom, bbox)

	features := make([]GeoJSONFeature, len(clusters))
	for i, c := range clusters {
		features[i] = GeoJSONFeature{
			Type: "Feature",
			Geometry: GeoJSONPoint{
				Type:        "Point",
				Coordinates: [2]float64{c.Lon, c.Lat},
			},
			Properties: ClusterProperties{
				Count:       c.Count,
				IDs:         c.IDs,
				ThumbnailID: c.ThumbnailID,
			},
		}
	}

	writeJSON(w, http.StatusOK, GeoJSONFeatureCollection{
		Type:     "FeatureCollection",
		Features: features,
	})
}

func (h *mapHandler) getGPSCount(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("albumId")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		badRequest(w, "albumId must be an integer")
		return
	}
	mediaRepo := repositories.NewMediaRepo(h.store.DB())
	count, err := mediaRepo.CountGPSMedia(&id)
	if err != nil {
		internalError(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"count": count})
}
