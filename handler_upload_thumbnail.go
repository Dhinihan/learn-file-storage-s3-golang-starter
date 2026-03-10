package main

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	maxMem := 10 << 20
	if err := r.ParseMultipartForm(int64(maxMem)); err != nil {
		respondWithError(w, http.StatusBadRequest, "Error processing request", err)
		return
	}
	file, fHeader, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Error processing request", err)
		return
	}
	cType := fHeader.Header.Get("Content-Type")
	vidMeta, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(
			w,
			http.StatusInternalServerError,
			"Error fetching video metadata",
			err,
		)
		return
	}
	if vidMeta.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", nil)
		return
	}
	ext, err := mime.ExtensionsByType(cType)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Error processing request", err)
		return
	}
	filename := videoIDString + ext[0]
	path := filepath.Join(cfg.assetsRoot, filename)
	wFile, err := os.Create(path)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error creating file", err)
		return
	}
	if _, err := io.Copy(wFile, file); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error creating file", err)
		return
	}
	fileURL := fmt.Sprintf("http://localhost:%s/assets/%s", cfg.port, filename)
	vidMeta.ThumbnailURL = &fileURL
	if err := cfg.db.UpdateVideo(vidMeta); err != nil {
		respondWithError(
			w,
			http.StatusInternalServerError,
			"Error updating the video",
			err,
		)
		return
	}

	respondWithJSON(w, http.StatusOK, vidMeta)
}
