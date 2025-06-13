package api

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-openapi/runtime/middleware"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	ops_content "peertech.de/axion/api/restapi/operations/content"
)

const (
	maxUploadSize = 1024 * 1024 * 1024 // 1GB limit
)

func (api *API) handleUpload(params ops_content.UploadParams) middleware.Responder {
	scopedLog := log.With().
		Str("handler", "handleUpload").
		Str("path", params.Path).
		Bool("recursive", params.Recursive != nil && *params.Recursive).
		Logger()

	if params.Path == "" {
		return ops_content.NewUploadBadRequest().
			WithPayload(newAPIError(http.StatusBadRequest, WithMessage("Missing file path")))
	}

	// Check content size
	if params.HTTPRequest.ContentLength > maxUploadSize {
		return ops_content.NewUploadRequestEntityTooLarge().
			WithPayload(newAPIError(http.StatusRequestEntityTooLarge, WithMessage("Upload too large")))
	}

	recursive := params.Recursive != nil && *params.Recursive

	// Check for path conflicts
	if fi, err := os.Stat(params.Path); err == nil {
		if fi.IsDir() && !recursive {
			return ops_content.NewUploadConflict().
				WithPayload(newAPIError(http.StatusConflict, WithMessage("Path is a directory, use recursive=true for directory uploads")))
		}
		if !fi.IsDir() && recursive {
			return ops_content.NewUploadConflict().
				WithPayload(newAPIError(http.StatusConflict, WithMessage("Path is a file, use recursive=false for file uploads")))
		}
	}

	if recursive {
		return api.handleDirectoryUpload(scopedLog, params)
	}

	return api.handleFileUpload(scopedLog, params)

}

func (api *API) handleDirectoryUpload(scopedLog zerolog.Logger, params ops_content.UploadParams) middleware.Responder {
	existed := true
	if _, err := os.Stat(params.Path); os.IsNotExist(err) {
		existed = false
	}

	// Create target directory if it doesn't exist
	if err := os.MkdirAll(params.Path, 0755); err != nil {
		scopedLog.Error().Err(err).Msg("Failed to create target directory")
		return ops_content.NewUploadInternalServerError().
			WithPayload(newAPIError(http.StatusInternalServerError, WithMessage("Failed to create target directory")))
	}

	// Extract tar.gz archive to directory
	if err := api.extractTarArchive(params.Content, params.Path); err != nil {
		scopedLog.Error().Err(err).Msg("Failed to extract archive")
		return ops_content.NewUploadUnprocessableEntity().
			WithPayload(newAPIError(http.StatusUnprocessableEntity, WithMessage("Failed to extract archive")))
	}

	if existed {
		return ops_content.NewUploadNoContent()
	} else {
		return ops_content.NewUploadCreated()
	}
}

func (api *API) extractTarArchive(src io.ReadCloser, destDir string) error {
	defer src.Close()

	// Create gzip reader
	gzr, err := gzip.NewReader(src)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzr.Close()

	// Create tar reader
	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar entry: %w", err)
		}

		// Calculate destination path
		destPath := filepath.Join(destDir, header.Name)

		// Pevent path traversal
		if !strings.HasPrefix(destPath, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path in archive: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			// Create directory
			if err := os.MkdirAll(destPath, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", destPath, err)
			}

		case tar.TypeReg:
			// Extract regular file
			if err := api.extractTarFile(tr, destPath, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("failed to extract file %s: %w", header.Name, err)
			}

		case tar.TypeSymlink:
			// Create symbolic link
			if err := api.createSymlink(header.Linkname, destPath); err != nil {
				return fmt.Errorf("failed to create symlink %s: %w", header.Name, err)
			}

		default:
			// Skip unsupported file types
			log.Warn().
				Str("file", header.Name).
				Str("type", string(rune(header.Typeflag))).
				Msg("Skipping unsupported file type in archive")
		}
	}

	return nil
}

func (api *API) handleFileUpload(scopedLog zerolog.Logger, params ops_content.UploadParams) middleware.Responder {
	existed := true
	if _, err := os.Stat(params.Path); os.IsNotExist(err) {
		existed = false
	}

	// Create parent directories if they don't exist
	if err := os.MkdirAll(filepath.Dir(params.Path), 0755); err != nil {
		scopedLog.Error().Err(err).Msg("Failed to create parent directories")
		return ops_content.NewUploadInternalServerError().
			WithPayload(newAPIError(http.StatusInternalServerError, WithMessage("Failed to create parent directories")))
	}

	// Extract single file from tar.gz archive
	if err := api.extractSingleFileFromTar(params.Content, params.Path); err != nil {
		scopedLog.Error().Err(err).Msg("Failed to extract file from archive")
		return ops_content.NewUploadUnprocessableEntity().
			WithPayload(newAPIError(http.StatusUnprocessableEntity, WithMessage("Failed to extract file from archive")))
	}

	if existed {
		return ops_content.NewUploadNoContent()
	} else {
		return ops_content.NewUploadCreated()
	}
}

func (api *API) extractSingleFileFromTar(src io.ReadCloser, destPath string) error {
	defer src.Close()

	// Create gzip reader
	gzr, err := gzip.NewReader(src)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzr.Close()

	// Create tar reader
	tr := tar.NewReader(gzr)

	// Read entry
	header, err := tr.Next()
	if err != nil {
		return fmt.Errorf("failed to read tar entry: %w", err)
	}

	// Verify it's a regular file
	if header.Typeflag != tar.TypeReg {
		return fmt.Errorf("archive must contain a regular file, found type: %c", header.Typeflag)
	}

	// Extract the file
	if err := api.extractTarFile(tr, destPath, os.FileMode(header.Mode)); err != nil {
		return err // No need to wrap, extractTarFile has good errors
	}

	// Check for extra data after the first file
	if _, err := tr.Next(); err != io.EOF {
		// Clean up the partially created file on error
		os.Remove(destPath)
		return fmt.Errorf("archive must contain only one file")
	}

	return nil
}

func (api *API) extractTarFile(tarReader *tar.Reader, destPath string, mode os.FileMode) error {
	// Create parent directory
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	// Create destination file
	destFile, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer destFile.Close()

	// Copy content
	if _, err := io.Copy(destFile, tarReader); err != nil {
		return fmt.Errorf("failed to write file content: %w", err)
	}

	return nil
}

func (api *API) createSymlink(target, linkPath string) error {
	// Create parent directory
	if err := os.MkdirAll(filepath.Dir(linkPath), 0755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	// Remove existing file/link if it exists
	if err := os.Remove(linkPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove existing file: %w", err)
	}

	// Create symbolic link
	if err := os.Symlink(target, linkPath); err != nil {
		return fmt.Errorf("failed to create symlink: %w", err)
	}

	return nil
}
