package main

import (
	"crypto/rand"
	"encoding/base64"
	"io"
	"mime"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {

	maxMemoryUpload := 1 << 30
	videoIDString := r.PathValue("videoID")
	videoId, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid video id", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", err)
		return
	}
	userId, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", err)
		return
	}
	vidMeta, err := cfg.db.GetVideo(videoId)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Video not found", err)
		return
	}
	if vidMeta.UserID != userId {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", nil)
		return
	}
	if err := r.ParseMultipartForm(int64(maxMemoryUpload)); err != nil {
		respondWithError(w, http.StatusBadRequest, "Undecodable request", err)
		return
	}
	file, fHeader, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "video is required", err)
		return
	}
	defer file.Close()
	mType, _, err := mime.ParseMediaType(fHeader.Header.Get("Content-Type"))
	if err != nil || mType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Video must be a mp4", err)
		return
	}
	tmpVideo, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed saving video", err)
		return
	}
	defer os.Remove(tmpVideo.Name())
	defer tmpVideo.Close()
	if _, err := io.Copy(tmpVideo, file); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed saving video", err)
		return
	}
	if _, err := tmpVideo.Seek(0, io.SeekStart); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed saving video", err)
		return
	}
	rnd := make([]byte, 32)
	rand.Read(rnd)
	filehash := base64.RawURLEncoding.EncodeToString(rnd) + ".mp4"

	if _, err := cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &filehash,
		Body:        tmpVideo,
		ContentType: &mType,
	}); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed saving video", err)
		return
	}
	videoURL := "https://tubely-83183.s3.sa-east-1.amazonaws.com/" + filehash
	vidMeta.VideoURL = &videoURL
	if err := cfg.db.UpdateVideo(vidMeta); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed saving video", err)
		return
	}

}
