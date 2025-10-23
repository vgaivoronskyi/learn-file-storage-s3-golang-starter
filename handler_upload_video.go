package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"os/exec"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

type Stream struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type VideoJson struct {
	Streams []Stream `json:"streams"`
}

func getVideoAspectRatio(filePath string) (string, error) {
	cmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return "", err
	}
	var vj VideoJson
	err = json.Unmarshal(out.Bytes(), &vj)
	if err != nil {
		return "", err
	}
	aspect := float64(vj.Streams[0].Width) / float64(vj.Streams[0].Height)
	if aspect > 1.7 && aspect < 1.85 {
		return "16:9", nil
	} else if aspect > 0.5 && aspect < 0.6 {
		return "9:16", nil
	} else {
		return "other", nil
	}
}

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<30)

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

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Couldn't get video", err)
		return
	}
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "You can't upload this video", err)
		return
	}

	file, header, err := r.FormFile("video")
	defer file.Close()

	mediaType := header.Header.Get("Content-Type")
	t, _, err := mime.ParseMediaType(mediaType)
	if err != nil || t != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "File type is not MP4", nil)
		return
	}

	f, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	defer os.Remove(f.Name())

	if _, err := io.Copy(f, file); err != nil {
		log.Fatal(err)
	}
	aspect, err := getVideoAspectRatio(f.Name())
	var prefix string
	if aspect == "16:9" {
		prefix = "landscape/"
	} else if aspect == "9:16" {
		prefix = "portrait/"
	} else {
		prefix = "other/"
	}

	f.Seek(0, io.SeekStart)

	key := make([]byte, 32)
	rand.Read(key)
	newFileName := hex.EncodeToString(key) + ".mp4"
	video_url := cfg.s3Bucket + "," + key

    processedVideoPath, err := processVideoForFastStart(f.Name())
    if err != nil {
        log.Fatal(err)
    }
    newFile,err := os.Open(processedVideoPath)
    if err != nil {
        log.Fatal(err)
    }
    defer newFile.Close()

	cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket:      aws.String(cfg.s3Bucket),
		Key:         aws.String(prefix + newFileName),
		Body:        newFile,
		ContentType: aws.String(t),
	})

	newVideoURL := "https://" + cfg.s3Bucket + ".s3." + cfg.s3Region + ".amazonaws.com/" + prefix + newFileName
	video.VideoURL = &newVideoURL
	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Can't update video", err)
		return
	}

}

func processVideoForFastStart(filePath string) (string, error) {
    tempOutput := filePath + ".processing"
    cmd := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", tempOutput)
	err := cmd.Run()
	if err != nil {
		return "", err
	}
    return tempOutput, nil
}

func generatePresignedURL(s3Client *s3.Client, bucket, key string, expireTime time.Duration) (string, error) {
    presignedClient := s3.NewPresignClient(s3Client)
    s3.PresignGetObject()

}
