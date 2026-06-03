package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/USERNAME/visiorama/internal/index"
	"github.com/USERNAME/visiorama/internal/index/repositories"
)

type albumsHandler struct {
	store *index.Store
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
