package api

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/user"
	"strconv"
	"syscall"
	"time"

	"github.com/go-openapi/runtime/middleware"
	"github.com/rs/zerolog/log"

	"peertech.de/axion/api/models"
	ops_files "peertech.de/axion/api/restapi/operations/files"
)

func (api *API) handleGetFileProperties(params ops_files.GetFilePropertiesParams) middleware.Responder {
	scopedLog := log.With().
		Str("handler", "handleGetFileProperties").
		Str("path", params.Path).
		Logger()

	if params.Path == "" {
		return middleware.Error(http.StatusBadRequest, "File path cannot be empty")
	}

	fi, err := os.Stat(params.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return ops_files.NewGetFilePropertiesNotFound().WithPayload(newAPIError(http.StatusNotFound))
		}

		scopedLog.Error().Err(err).Msg("Failed to stat file")
		return ops_files.NewGetFilePropertiesInternalServerError().
			WithPayload(newAPIError(http.StatusInternalServerError, WithMessage("Failed to stat file")))
	}

	checksum, err := calculateFileChecksum(params.Path)
	if err != nil {
		scopedLog.Error().Err(err).Msg("Failed to calculate file checksum")
		return ops_files.NewGetFilePropertiesInternalServerError().
			WithPayload(newAPIError(http.StatusInternalServerError, WithMessage("Failed to calculate file checksum")))
	}

	stat := fi.Sys().(*syscall.Stat_t)
	owner, err := user.LookupId(fmt.Sprint(stat.Uid))
	if err != nil {
		scopedLog.Error().Err(err).Msg("Failed to lookup user id")
		return ops_files.NewGetFilePropertiesInternalServerError().
			WithPayload(newAPIError(http.StatusInternalServerError, WithMessage("Failed to lookup user id")))
	}

	group, err := user.LookupGroupId(fmt.Sprint(stat.Gid))
	if err != nil {
		scopedLog.Error().Err(err).Msg("Failed to lookup group id")
		return ops_files.NewGetFilePropertiesInternalServerError().
			WithPayload(newAPIError(http.StatusInternalServerError, WithMessage("Failed to lookup group id")))
	}

	file := &models.FileProperties{
		Mode:     encodeFileMode(fi.Mode()),
		Owner:    owner.Username,
		Group:    group.Name,
		Checksum: checksum,
	}

	etag := generateFileETag(fi)
	return ops_files.NewGetFilePropertiesOK().WithETag(etag).WithPayload(file)
}

func (api *API) handlePutFile(params ops_files.PutFileParams) middleware.Responder {
	scopedLog := log.With().
		Str("handler", "handlePutFile").
		Str("path", params.Path).
		Logger()

	if params.Path == "" {
		return ops_files.NewPutFileBadRequest().
			WithPayload(newAPIError(http.StatusBadRequest, WithMessage("File path cannot be empty")))
	}

	var (
		mode     *os.FileMode
		uid, gid *int
	)

	if params.Properties != nil && params.Properties.Mode != "" {
		v, err := decodeFileMode(params.Properties.Mode)
		if err != nil {
			return ops_files.NewPutFileBadRequest().
				WithPayload(newAPIError(http.StatusBadRequest, WithMessage("Invalid mode")))
		}
		mode = &v
	}

	if params.Properties != nil && params.Properties.Owner != "" {
		u, err := user.Lookup(params.Properties.Owner)
		if err != nil {
			var uue *user.UnknownUserError
			if errors.As(err, &uue) {
				return ops_files.NewPutFileBadRequest().
					WithPayload(newAPIError(http.StatusBadRequest, WithMessage("Invalid owner")))
			} else {
				scopedLog.Error().Err(err).Msg("Failed to lookup owner")
				return ops_files.NewPutFileInternalServerError().
					WithPayload(newAPIError(http.StatusInternalServerError, WithMessage("Failed to lookup owner")))
			}
		}

		id, _ := strconv.Atoi(u.Uid)
		uid = &id
	}

	if params.Properties != nil && params.Properties.Group != "" {
		g, err := user.LookupGroup(params.Properties.Group)
		if err != nil {
			var uge *user.UnknownGroupError
			if errors.As(err, &uge) {
				return ops_files.NewPutFileBadRequest().
					WithPayload(newAPIError(http.StatusBadRequest, WithMessage("Invalid group")))
			} else {
				scopedLog.Error().Err(err).Msg("Failed to lookup group")
				return ops_files.NewPutFileInternalServerError().
					WithPayload(newAPIError(http.StatusInternalServerError, WithMessage("Failed to lookup group")))
			}
		}

		id, _ := strconv.Atoi(g.Gid)
		gid = &id
	}

	fi, err := os.Stat(params.Path)
	fileExists := err == nil

	ifMatch := params.HTTPRequest.Header.Get("If-Match")
	if ifMatch != "" {
		if !fileExists {
			return ops_files.NewPutFilePreconditionFailed().
				WithPayload(newAPIError(http.StatusPreconditionFailed, WithMessage("File does not exist for conditional update")))
		}

		currentETag := generateFileETag(fi)
		if ifMatch != currentETag {
			return ops_files.NewPutFileConflict().
				WithPayload(newAPIError(http.StatusConflict, WithMessage("ETag mismatch")))
		}
	} else if fileExists {
		// File exists but no If-Match header sent
		return ops_files.NewPutFilePreconditionRequired().
			WithPayload(newAPIError(http.StatusPreconditionRequired, WithMessage("Missing If-Match header")))
	}

	created, err := putFile(params.Path, mode, uid, gid)
	if err != nil {
		var oe *OpError
		if errors.As(err, &oe) {
			// putFile only returns http.StatusInternalServerError
			return ops_files.NewPutFileInternalServerError().
				WithPayload(newAPIError(oe.Code, WithMessage(oe.Msg)))
		} else {
			return ops_files.NewPutFileInternalServerError().
				WithPayload(newAPIError(http.StatusInternalServerError, WithMessage(err.Error())))
		}
	}

	if created {
		return ops_files.NewPutFileCreated()
	}

	return ops_files.NewPutFileNoContent()
}

func (api *API) handleDeleteFile(params ops_files.DeleteFileParams) middleware.Responder {
	scopedLog := log.With().
		Str("handler", "handleDeleteFile").
		Str("path", params.Path).
		Logger()

	if params.Path == "" {
		return ops_files.NewPutFileBadRequest().
			WithPayload(newAPIError(http.StatusBadRequest, WithMessage("File path cannot be empty")))
	}

	fi, err := os.Stat(params.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ops_files.NewDeleteFileNoContent()
		}

		scopedLog.Error().Err(err).Msg("Failed to stat file")
		return ops_files.NewDeleteFileInternalServerError().
			WithPayload(newAPIError(http.StatusInternalServerError, WithMessage("Failed to stat file")))
	}

	ifMatch := params.HTTPRequest.Header.Get("If-Match")
	if ifMatch == "" {
		return ops_files.NewDeleteFilePreconditionRequired().
			WithPayload(newAPIError(http.StatusPreconditionRequired, WithMessage("Missing If-Match header")))
	}

	currentETag := generateFileETag(fi)
	if ifMatch != currentETag {
		return ops_files.NewDeleteFileConflict().
			WithPayload(newAPIError(http.StatusConflict, WithMessage("ETag mismach")))
	}

	err = os.Remove(params.Path)
	switch {
	case err == nil:
	case errors.Is(err, os.ErrPermission):
		return ops_files.NewDeleteFileForbidden().
			WithPayload(newAPIError(http.StatusForbidden, WithMessage("Permission denied")))
	case errors.Is(err, os.ErrNotExist):
		// For some reason the file was deleted between the last os.Stat call just catch
		// the error so it doesn't get caught by default
	default:
		scopedLog.Error().Err(err).Msg("Failed to delete file")
		return ops_files.NewDeleteFileInternalServerError().
			WithPayload(newAPIError(http.StatusInternalServerError, WithMessage("Failed to delete file")))
	}

	return ops_files.NewDeleteFileNoContent()
}

func putFile(path string, mode *os.FileMode, uid, gid *int) (created bool, err error) {
	fi, err := os.Stat(path)
	fileExists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, newOpError(http.StatusInternalServerError, "Failed to stat file", err)
	}

	if !fileExists {
		if err := os.WriteFile(path, []byte{}, 0644); err != nil {
			return false, newOpError(http.StatusInternalServerError, "Failed to create file", err)
		}
		created = true

		fi, err = os.Stat(path)
		if err != nil {
			return false, newOpError(http.StatusInternalServerError, "Failed to stat file after creation", err)
		}
	}

	stat := fi.Sys().(*syscall.Stat_t)
	currentMode := fi.Mode().Perm()
	currentUID := int(stat.Uid)
	currentGID := int(stat.Gid)

	if mode != nil && mode != &currentMode {
		if err := os.Chmod(path, *mode); err != nil {
			return created, newOpError(http.StatusInternalServerError, "Failed to chmod file", err)
		}
	}

	var needChown bool
	targetUID := currentUID // backup current uid in case only one is given
	targetGID := currentGID // backup current gid in case only one is given

	if uid != nil && *uid != currentUID {
		targetUID = *uid
		needChown = true
	}

	if gid != nil && *gid != currentGID {
		targetGID = *gid
		needChown = true
	}

	if needChown {
		if err := os.Chown(path, targetUID, targetGID); err != nil {
			return created, newOpError(http.StatusInternalServerError, "Failed to chown file", err)
		}
	}

	return created, nil
}

// encodeFileMode encodes a file mode to an octal string
func encodeFileMode(mode os.FileMode) string {
	return fmt.Sprintf("0%o", mode.Perm())
}

// decodeFileMode decodes an octal string to a file mode
func decodeFileMode(modeString string) (os.FileMode, error) {
	mode, err := strconv.ParseUint(modeString, 8, 32)
	if err != nil {
		return 0, err
	}
	return os.FileMode(mode), nil
}

func generateFileETag(fi os.FileInfo) string {
	stat, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}

	data := fmt.Sprintf("%v:%d:%d:%s",
		fi.Mode().Perm(),
		stat.Uid,
		stat.Gid,
		fi.ModTime().UTC().Format(time.RFC3339Nano),
	)
	sum := sha256.Sum256([]byte(data))
	return `W/"` + hex.EncodeToString(sum[:]) + `"`
}

func calculateFileChecksum(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
