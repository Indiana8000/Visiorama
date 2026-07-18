package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/Indiana8000/visiorama/internal/app"
	"github.com/Indiana8000/visiorama/internal/index"
	"github.com/Indiana8000/visiorama/internal/index/repositories"
	"github.com/Indiana8000/visiorama/internal/util"
)

type albumsHandler struct {
	cfg    *app.Config
	store  *index.Store
	runner orphanTrigger
}

func (h *albumsHandler) maybeStartOrphanScan() {
	if h.runner == nil || h.runner.IsRunning() {
		return
	}
	scanRepo := repositories.NewScanRepo(h.store.DB())
	scanID := fmt.Sprintf("scan-orphan-%d", time.Now().UnixMilli())
	_ = scanRepo.Create(&repositories.ScanJob{ID: scanID, Mode: "orphan", Status: "queued"})
	_ = h.runner.TriggerAsync(scanID, "orphan", "")
}

func (h *albumsHandler) getRoot(w http.ResponseWriter, r *http.Request) {
	page, pageSize, ok := parsePagination(w, r)
	if !ok {
		return
	}
	albumRepo := repositories.NewAlbumsRepo(h.store.DB())
	album, err := albumRepo.GetRoot()
	if err != nil {
		internalError(w)
		return
	}
	// DB not yet scanned — return empty root so the UI can show a scan prompt
	if album == nil {
		writeJSON(w, http.StatusOK, AlbumResponse{
			Album:       Album{ID: 0, RelativePath: "", Name: "Visiorama"},
			Breadcrumbs: []Breadcrumb{},
			ChildAlbums: []AlbumTile{},
			Media:       []MediaSummary{},
			Page:        PageInfo{Page: 1, PageSize: pageSize, TotalItems: 0, TotalPages: 1},
		})
		return
	}
	h.buildAndWrite(w, albumRepo, album, page, pageSize)
}

func (h *albumsHandler) getByID(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("albumId")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		badRequest(w, "albumId must be an integer")
		return
	}
	page, pageSize, ok := parsePagination(w, r)
	if !ok {
		return
	}
	albumRepo := repositories.NewAlbumsRepo(h.store.DB())
	album, err := albumRepo.GetByID(id)
	if err != nil || album == nil {
		notFound(w)
		return
	}
	h.buildAndWrite(w, albumRepo, album, page, pageSize)
}

func (h *albumsHandler) getByPath(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		badRequest(w, "path query parameter is required")
		return
	}
	page, pageSize, ok := parsePagination(w, r)
	if !ok {
		return
	}
	albumRepo := repositories.NewAlbumsRepo(h.store.DB())
	album, err := albumRepo.GetByPath(path)
	if err != nil || album == nil {
		notFound(w)
		return
	}
	h.buildAndWrite(w, albumRepo, album, page, pageSize)
}

func (h *albumsHandler) buildAndWrite(w http.ResponseWriter, albumRepo *repositories.AlbumsRepo, album *repositories.Album, page, pageSize int) {
	// Non-root album: check the directory still exists on disk.
	if album.RelativePath != "" && h.cfg != nil {
		absPath := filepath.Join(h.cfg.Library.RootPath, filepath.FromSlash(album.RelativePath))
		if _, err := os.Stat(absPath); os.IsNotExist(err) {
			if h.runner != nil {
				h.maybeStartOrphanScan()
			}
			notFound(w)
			return
		}
	}

	mediaRepo := repositories.NewMediaRepo(h.store.DB())

	// breadcrumbs
	breadcrumbAlbums, err := albumRepo.Breadcrumbs(album.ID)
	if err != nil {
		internalError(w)
		return
	}
	breadcrumbs := make([]Breadcrumb, len(breadcrumbAlbums))
	for i, b := range breadcrumbAlbums {
		id := b.ID
		breadcrumbs[i] = Breadcrumb{AlbumID: &id, Name: b.Name, RelativePath: b.RelativePath}
	}
	if len(breadcrumbs) > 0 {
		breadcrumbs[0].AlbumID = nil // root has no albumId in breadcrumb
	}

	// child albums
	children, err := albumRepo.ListChildren(album.ID)
	if err != nil {
		internalError(w)
		return
	}
	sort.Slice(children, func(i, j int) bool {
		return util.NaturalLess(children[i].Name, children[j].Name)
	})
	childTiles := make([]AlbumTile, len(children))
	for i, c := range children {
		parentID := c.ParentAlbumID
		tile := AlbumTile{
			Album: Album{
				ID:                  c.ID,
				RelativePath:        c.RelativePath,
				Name:                c.Name,
				ParentAlbumID:       parentID,
				MediaCountDirect:    c.MediaCountDirect,
				MediaCountRecursive: c.MediaCountRecursive,
				ChildAlbumCount:     c.ChildAlbumCount,
			},
		}
		if coverID, _ := albumRepo.CoverMediaID(c.ID); coverID != nil {
			url := fmt.Sprintf("/api/media/%d/thumbnail", *coverID)
			tile.CoverMediaID = coverID
			tile.CoverThumbnailURL = &url
		}
		childTiles[i] = tile
	}

	// paginated media
	totalItems, err := mediaRepo.CountByAlbum(album.ID)
	if err != nil {
		internalError(w)
		return
	}
	offset := (page - 1) * pageSize
	mediaRows, err := mediaRepo.ListByAlbum(album.ID, offset, pageSize)
	if err != nil {
		internalError(w)
		return
	}
	sort.Slice(mediaRows, func(i, j int) bool {
		return util.NaturalLess(mediaRows[i].Filename, mediaRows[j].Filename)
	})
	mediaSummaries := make([]MediaSummary, len(mediaRows))
	for i, m := range mediaRows {
		mediaSummaries[i] = repoMediaToSummary(&m)
	}

	totalPages := (totalItems + pageSize - 1) / pageSize
	if totalPages == 0 {
		totalPages = 1
	}

	parentID := album.ParentAlbumID
	writeJSON(w, http.StatusOK, AlbumResponse{
		Album: Album{
			ID:                  album.ID,
			RelativePath:        album.RelativePath,
			Name:                album.Name,
			ParentAlbumID:       parentID,
			MediaCountDirect:    album.MediaCountDirect,
			MediaCountRecursive: album.MediaCountRecursive,
			ChildAlbumCount:     album.ChildAlbumCount,
		},
		Breadcrumbs: breadcrumbs,
		ChildAlbums: childTiles,
		Media:       mediaSummaries,
		Page: PageInfo{
			Page:       page,
			PageSize:   pageSize,
			TotalItems: totalItems,
			TotalPages: totalPages,
			HasNext:    page*pageSize < totalItems,
			HasPrev:    page > 1,
		},
	})
}

func (h *albumsHandler) albumsByMediaIDs(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []int64 `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.IDs) == 0 {
		badRequest(w, "ids must be a non-empty array of integers")
		return
	}
	if len(body.IDs) > 999 {
		body.IDs = body.IDs[:999]
	}
	albumRepo := repositories.NewAlbumsRepo(h.store.DB())
	matches, err := albumRepo.AlbumsByMediaIDs(body.IDs)
	if err != nil {
		internalError(w)
		return
	}
	result := make([]AlbumMatch, len(matches))
	for i, m := range matches {
		am := AlbumMatch{
			ID:           m.ID,
			RelativePath: m.RelativePath,
			Name:         m.Name,
			MatchCount:   m.MatchCount,
		}
		if coverID, _ := albumRepo.CoverMediaID(m.ID); coverID != nil {
			url := fmt.Sprintf("/api/media/%d/thumbnail", *coverID)
			am.CoverMediaID = coverID
			am.CoverThumbnailURL = &url
		}
		result[i] = am
	}
	writeJSON(w, http.StatusOK, result)
}

func parsePagination(w http.ResponseWriter, r *http.Request) (page, pageSize int, ok bool) {
	page = 1
	pageSize = 100

	if v := r.URL.Query().Get("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			badRequest(w, "page must be a positive integer")
			return 0, 0, false
		}
		page = n
	}
	if v := r.URL.Query().Get("pageSize"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 500 {
			badRequest(w, "pageSize must be between 1 and 500")
			return 0, 0, false
		}
		pageSize = n
	}
	return page, pageSize, true
}
