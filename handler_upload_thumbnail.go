package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"

	//	"encoding/base64"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"

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

	const maxMemory = 10 << 20
	if err := r.ParseMultipartForm(maxMemory); err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse multipart form", err)
		return
	}

	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	mediaType := header.Header.Get("Content-Type")
	if mediaType == "" {
		respondWithError(w, http.StatusBadRequest, "Missing Content-Type for thumbnail", nil)
		return
	}

	key := make([]byte, 32)
	rand.Read(key)

	newFileName := base64.RawURLEncoding.EncodeToString(key) + "." + strings.Split(mediaType, "/")[1]
	filePath := filepath.Join(cfg.assetsRoot, newFileName)
	newFile, err := os.Create(filePath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to create file", err)
		return
	}
	if _, err := io.Copy(newFile, file); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable copy to new file", err)
		return
	}

	fileURL := "http://localhost" + ":" + cfg.port + "/assets/" + newFileName
	// 	imageData, err := io.ReadAll(file)
	// 	if err != nil {
	// 		respondWithError(w, http.StatusBadRequest, "Unable to read thumbnail data", err)
	// 		return
	// 	}
	//
	//     dataString := base64.StdEncoding.EncodeToString(imageData)
	//    dataURL  := "data:" + mediaType + ";base64," + dataString

	meta, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Couldn't get video", err)
		return
	}
	if meta.ID == uuid.Nil {
		respondWithError(w, http.StatusNotFound, "Video not found", nil)
		return
	}
	if meta.UserID != userID {
		respondWithError(w, http.StatusForbidden, "You're not the video owner", nil)
		return
	}

	meta.ThumbnailURL = &fileURL

	err = cfg.db.UpdateVideo(meta)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Can't update video", err)
		return
	}

	respondWithJSON(w, http.StatusOK, meta)
}
