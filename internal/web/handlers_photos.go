// internal/web/handlers_photos.go
package web

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"homesite/internal/photos"
	"homesite/views"
)

const (
	galleryPageSize      = 60
	multipartMemoryLimit = 10 << 20 // 10MB held in memory before spilling to temp files
)

func (s *Server) handlePhotosPage(w http.ResponseWriter, r *http.Request) {
	page := parsePage(r)
	gallery, hasMore := s.listGalleryPage(r, page)

	showWarning := false
	if used, err := s.photosStore.DiskUsageBytes(s.cfg.UploadsDir); err == nil && s.cfg.Photos.MaxTotalBytes > 0 {
		percent := used * 100 / s.cfg.Photos.MaxTotalBytes
		showWarning = percent >= int64(s.cfg.Photos.WarnAtPercent)
	}

	render(w, r, views.PhotosPage(gallery, page, page > 1, hasMore, showWarning))
}

func (s *Server) handlePhotosUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(multipartMemoryLimit); err != nil {
		http.Error(w, "could not parse upload", http.StatusBadRequest)
		return
	}
	files := r.MultipartForm.File["photos"]
	result := views.UploadResult{Total: len(files)}
	ip := clientIP(r)

	for _, fh := range files {
		f, err := fh.Open()
		if err != nil {
			result.Failures = append(result.Failures, views.UploadFailure{Filename: fh.Filename, Message: "couldn't read that file"})
			continue
		}
		_, err = s.photosIngester.Ingest(r.Context(), f, "gallery", ip)
		f.Close()
		if err != nil {
			log.Printf("web: photo upload rejected (%s): %v", fh.Filename, err)
			result.Failures = append(result.Failures, views.UploadFailure{Filename: fh.Filename, Message: uploadErrorMessage(err)})
			continue
		}
		result.Succeeded++
	}

	gallery, hasMore := s.listGalleryPage(r, 1)
	render(w, r, views.PhotosContent(&result, gallery, 1, false, hasMore))
}

// handlePhotosNew backs the gallery's live polling: the grid on page 1
// hits GET /photos/new?after=<id> every 10s (see photos.templ's
// #photo-grid hx-get) with the id of whatever photo currently renders
// first, and this returns anything newer for the client to prepend.
func (s *Server) handlePhotosNew(w http.ResponseWriter, r *http.Request) {
	afterID, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	newPhotos, err := s.photosStore.ListAfter(r.Context(), afterID, galleryPageSize)
	if err != nil {
		log.Printf("web: listing new photos after %d: %v", afterID, err)
		newPhotos = nil
	}
	render(w, r, views.NewPhotos(newPhotos))
}

func (s *Server) listGalleryPage(r *http.Request, page int) (gallery []photos.Photo, hasMore bool) {
	offset := (page - 1) * galleryPageSize
	rows, err := s.photosStore.List(r.Context(), galleryPageSize+1, offset)
	if err != nil {
		log.Printf("web: listing photos: %v", err)
		return nil, false
	}
	if len(rows) > galleryPageSize {
		return rows[:galleryPageSize], true
	}
	return rows, false
}

func parsePage(r *http.Request) int {
	n, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || n < 1 {
		return 1
	}
	return n
}

func uploadErrorMessage(err error) string {
	switch {
	case errors.Is(err, photos.ErrNotAnImage):
		return "that file isn't a photo — JPEG, PNG, WebP, or HEIC please"
	case errors.Is(err, photos.ErrTooLarge):
		return "that photo's too big — 25MB max"
	case errors.Is(err, photos.ErrDiskFull):
		return "photo storage is full — tell Kevin"
	case errors.Is(err, photos.ErrProcessing):
		return "couldn't process that photo, try another"
	default:
		return "upload failed"
	}
}
