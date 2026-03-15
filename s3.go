package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
)

func generatePresignedURL(s3Client *s3.Client, bucket, key string, expireTime time.Duration) (string, error) {
	psClient := s3.NewPresignClient(s3Client)
	psObject, err := psClient.PresignGetObject(context.Background(), &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	}, s3.WithPresignExpires(expireTime))
	if err != nil {
		return "", err
	}
	return psObject.URL, nil
}

func (cfg *apiConfig) dbVideoToSignedVideo(video database.Video) (database.Video, error) {
	if video.VideoURL == nil {
		return video, nil
	}
	urlParts := strings.Split(*video.VideoURL, ",")
	if len(urlParts) != 2 {
		return video, errors.New("url should have format <bucket>,<key>")
	}
	psUrl, err := generatePresignedURL(&cfg.s3Client, urlParts[0], urlParts[1], time.Hour)
	if err != nil {
		return video, err
	}
	video.VideoURL = &psUrl
	return video, nil
}
