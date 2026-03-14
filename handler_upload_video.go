package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"

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

	prcVideoPath, err := processVideoForFastStart(tmpVideo.Name())
	if err != nil {
		respondWithError(w, 500, "Error processing the video", err)
		return
	}
	defer os.Remove(prcVideoPath)
	prcVideo, err := os.Open(prcVideoPath)
	if err != nil {
		respondWithError(w, 500, "Error processing the video", err)
		return
	}
	defer prcVideo.Close()

	rnd := make([]byte, 32)
	rand.Read(rnd)
	ratio, err := getVideoAspectRatio(prcVideoPath)
	if err != nil {
		respondWithError(w, 500, "Error deciding the aspect ratio", err)
		return
	}
	folder := "other/"
	if ratio == "16:9" {
		folder = "landscape/"
	}
	if ratio == "9:16" {
		folder = "portrait/"
	}
	filehash := folder + base64.RawURLEncoding.EncodeToString(rnd) + ".mp4"

	if _, err := cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &filehash,
		Body:        prcVideo,
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

func getVideoAspectRatio(filePath string) (string, error) {
	cmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
	buff := bytes.NewBuffer([]byte{})
	cmd.Stdout = buff
	if err := cmd.Run(); err != nil {
		return "", err
	}
	var streamsData struct {
		Streams []struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"streams"`
	}
	rawData, err := io.ReadAll(buff)
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal(rawData, &streamsData); err != nil {
		return "", err
	}
	if len(streamsData.Streams) < 1 {
		return "", errors.New("No stream received")
	}
	stream := streamsData.Streams[0]
	ratio := (stream.Width * 1000) / stream.Height
	switch ratio {
	case 1777:
		return "16:9", nil
	case 562:
		return "9:16", nil
	}
	return "other", nil
}

func processVideoForFastStart(filePath string) (string, error) {
	output := filePath + ".processing"
	cmd := exec.Command(
		"ffmpeg",
		"-i",
		filePath,
		"-c",
		"copy",
		"-movflags",
		"faststart",
		"-f",
		"mp4",
		output,
	)
	buff := bytes.NewBuffer([]byte{})
	cmd.Stdout = buff
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return output, nil
}
