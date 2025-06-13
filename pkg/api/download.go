package api

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-openapi/runtime/middleware"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	ops_content "peertech.de/axion/api/restapi/operations/content"
)

func (api *API) handleDownload(params ops_content.DownloadParams) middleware.Responder {
	scopedLog := log.With().
		Str("handler", "handleDownload").
		Str("path", params.Path).
		Bool("recursive", params.Recursive != nil && *params.Recursive).
		Logger()

	if params.Path == "" {
		return ops_content.NewDownloadBadRequest().
			WithPayload(newAPIError(http.StatusBadRequest, WithMessage("Missing file path")))
	}

	fi, err := os.Stat(params.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ops_content.NewDownloadNotFound().
				WithPayload(newAPIError(http.StatusNotFound, WithMessage("File or directory not found")))
		}

		scopedLog.Error().Err(err).Msg("Failed to stat path")
		return ops_content.NewDownloadInternalServerError().
			WithPayload(newAPIError(http.StatusInternalServerError, WithMessage("Failed to access path")))
	}

	recursive := params.Recursive != nil && *params.Recursive
	if fi.IsDir() && !recursive {
		return ops_content.NewDownloadConflict().
			WithPayload(newAPIError(http.StatusConflict, WithMessage("Path is a directory, use recursive=true for directory downloads")))
	}

	if !fi.IsDir() && recursive {
		return ops_content.NewDownloadConflict().
			WithPayload(newAPIError(http.StatusConflict, WithMessage("Path is a file, use recursive=false for file downloads")))
	}

	return api.handleTarDownload(scopedLog, params.Path, fi.IsDir())
}

func (api *API) handleTarDownload(scopedLog zerolog.Logger, path string, isDirectory bool) middleware.Responder {
	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()

		// Create gzip writer
		gzw := gzip.NewWriter(pw)
		defer func() {
			if err := gzw.Close(); err != nil {
				scopedLog.Error().Err(err).Msg("Failed to close gzip writer")
			}
		}()

		// Create tar writer
		tw := tar.NewWriter(gzw)
		defer func() {
			if err := tw.Close(); err != nil {
				scopedLog.Error().Err(err).Msg("Failed to close tar writer")
			}
		}()

		var err error
		if isDirectory {
			err = api.addDirectoryToTar(tw, path, "")
		} else {
			err = api.addFileToTar(tw, path, filepath.Base(path))
		}

		if err != nil {
			scopedLog.Error().Err(err).Msg("Failed to create tar archive")
			pw.CloseWithError(err)
		}
	}()

	archiveType := "file"
	if isDirectory {
		archiveType = "directory"
	}

	filename := filepath.Base(path) + ".tar.gz"

	return ops_content.NewDownloadOK().
		WithPayload(pr).
		WithContentDisposition(fmt.Sprintf("attachment; filename=\"%s\"", filename)).
		WithXArchiveFormat("tar.gz").
		WithXArchiveType(archiveType)
}

func (api *API) addFileToTar(tarWriter *tar.Writer, filePath, tarPath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", filePath, err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file %s: %w", filePath, err)
	}

	// Create tar header
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return fmt.Errorf("failed to create tar header for %s: %w", filePath, err)
	}
	header.Name = tarPath

	// Write header
	if err := tarWriter.WriteHeader(header); err != nil {
		return fmt.Errorf("failed to write tar header for %s: %w", filePath, err)
	}

	// Copy file content
	if _, err := io.Copy(tarWriter, file); err != nil {
		return fmt.Errorf("failed to write file content for %s: %w", filePath, err)
	}

	return nil
}

func (api *API) addDirectoryToTar(tarWriter *tar.Writer, sourcePath, tarBasePath string) error {
	return filepath.Walk(sourcePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("walk error for %s: %w", path, err)
		}

		// Calculate relative path for tar entry
		relPath, err := filepath.Rel(sourcePath, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path for %s: %w", path, err)
		}

		// Use forward slashes in tar archives (cross-platform compatibility)
		tarPath := filepath.ToSlash(filepath.Join(tarBasePath, relPath))

		// Skip the root directory itself if it would create an empty entry
		if tarPath == "." || tarPath == "" {
			return nil
		}

		// Create tar header
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("failed to create tar header for %s: %w", path, err)
		}
		header.Name = tarPath

		// Write header
		if err := tarWriter.WriteHeader(header); err != nil {
			return fmt.Errorf("failed to write tar header for %s: %w", path, err)
		}

		// Write file content if it's a regular file
		if info.Mode().IsRegular() {
			file, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("failed to open file %s: %w", path, err)
			}
			defer file.Close()

			if _, err := io.Copy(tarWriter, file); err != nil {
				return fmt.Errorf("failed to write file content for %s: %w", path, err)
			}
		}

		return nil
	})
}
