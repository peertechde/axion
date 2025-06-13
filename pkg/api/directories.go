package api

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/user"
	"strconv"
	"syscall"

	"github.com/go-openapi/runtime/middleware"
	"github.com/rs/zerolog/log"

	"peertech.de/axion/api/models"
	ops_directories "peertech.de/axion/api/restapi/operations/directories"
)

func (api *API) handleGetDirectoryProperties(params ops_directories.GetDirectoryPropertiesParams) middleware.Responder {
	scopedLog := log.With().
		Str("handler", "handleGetDirectoryProperties").
		Str("path", params.Path).
		Logger()

	if params.Path == "" {
		return middleware.Error(http.StatusBadRequest, "Directory path cannot be empty")
	}

	fi, err := os.Stat(params.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return ops_directories.NewGetDirectoryPropertiesNotFound().WithPayload(newAPIError(http.StatusNotFound))
		}

		scopedLog.Error().Err(err).Msg("Failed to stat directory")
		return ops_directories.NewGetDirectoryPropertiesInternalServerError().
			WithPayload(newAPIError(http.StatusInternalServerError, WithMessage("Failed to stat directory")))
	}

	// Check if it's actually a directory
	if !fi.IsDir() {
		return ops_directories.NewGetDirectoryPropertiesBadRequest().
			WithPayload(newAPIError(http.StatusBadRequest, WithMessage("Path is not a directory")))
	}

	stat := fi.Sys().(*syscall.Stat_t)
	owner, err := user.LookupId(fmt.Sprint(stat.Uid))
	if err != nil {
		scopedLog.Error().Err(err).Msg("Failed to lookup user id")
		return ops_directories.NewGetDirectoryPropertiesInternalServerError().
			WithPayload(newAPIError(http.StatusInternalServerError, WithMessage("Failed to lookup user id")))
	}

	group, err := user.LookupGroupId(fmt.Sprint(stat.Gid))
	if err != nil {
		scopedLog.Error().Err(err).Msg("Failed to lookup group id")
		return ops_directories.NewGetDirectoryPropertiesInternalServerError().
			WithPayload(newAPIError(http.StatusInternalServerError, WithMessage("Failed to lookup group id")))
	}

	directory := &models.DirectoryProperties{
		Mode:  encodeFileMode(fi.Mode()),
		Owner: owner.Username,
		Group: group.Name,
	}

	etag := generateFileETag(fi)
	return ops_directories.NewGetDirectoryPropertiesOK().WithETag(etag).WithPayload(directory)
}

func (api *API) handlePutDirectory(params ops_directories.PutDirectoryParams) middleware.Responder {
	scopedLog := log.With().
		Str("handler", "handlePutDirectory").
		Str("path", params.Path).
		Logger()

	if params.Path == "" {
		return ops_directories.NewPutDirectoryBadRequest().
			WithPayload(newAPIError(http.StatusBadRequest, WithMessage("Directory path cannot be empty")))
	}

	var (
		mode     *os.FileMode
		uid, gid *int
	)

	if params.Properties != nil && params.Properties.Mode != "" {
		v, err := decodeFileMode(params.Properties.Mode)
		if err != nil {
			return ops_directories.NewPutDirectoryBadRequest().
				WithPayload(newAPIError(http.StatusBadRequest, WithMessage("Invalid mode")))
		}
		mode = &v
	}

	if params.Properties != nil && params.Properties.Owner != "" {
		u, err := user.Lookup(params.Properties.Owner)
		if err != nil {
			var uue *user.UnknownUserError
			if errors.As(err, &uue) {
				return ops_directories.NewPutDirectoryBadRequest().
					WithPayload(newAPIError(http.StatusBadRequest, WithMessage("Invalid owner")))
			} else {
				scopedLog.Error().Err(err).Msg("Failed to lookup owner")
				return ops_directories.NewPutDirectoryInternalServerError().
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
				return ops_directories.NewPutDirectoryBadRequest().
					WithPayload(newAPIError(http.StatusBadRequest, WithMessage("Invalid group")))
			} else {
				scopedLog.Error().Err(err).Msg("Failed to lookup group")
				return ops_directories.NewPutDirectoryInternalServerError().
					WithPayload(newAPIError(http.StatusInternalServerError, WithMessage("Failed to lookup group")))
			}
		}

		id, _ := strconv.Atoi(g.Gid)
		gid = &id
	}

	fi, err := os.Stat(params.Path)
	directoryExists := err == nil

	ifMatch := params.HTTPRequest.Header.Get("If-Match")
	if ifMatch != "" {
		if !directoryExists {
			return ops_directories.NewPutDirectoryPreconditionFailed().
				WithPayload(newAPIError(http.StatusPreconditionFailed, WithMessage("Directory does not exist for conditional update")))
		}

		currentETag := generateFileETag(fi)
		if ifMatch != currentETag {
			return ops_directories.NewPutDirectoryConflict().
				WithPayload(newAPIError(http.StatusConflict, WithMessage("ETag mismatch")))
		}
	} else if directoryExists {
		return ops_directories.NewPutDirectoryPreconditionRequired().
			WithPayload(newAPIError(http.StatusPreconditionRequired, WithMessage("Missing If-Match header")))
	}

	created, err := putDirectory(params.Path, mode, uid, gid)
	if err != nil {
		var oe *OpError
		if errors.As(err, &oe) {
			return ops_directories.NewPutDirectoryInternalServerError().
				WithPayload(newAPIError(oe.Code, WithMessage(oe.Msg)))
		} else {
			return ops_directories.NewPutDirectoryInternalServerError().
				WithPayload(newAPIError(http.StatusInternalServerError, WithMessage(err.Error())))
		}
	}

	if created {
		return ops_directories.NewPutDirectoryCreated()
	}

	return ops_directories.NewPutDirectoryNoContent()
}

func (api *API) handleDeleteDirectory(params ops_directories.DeleteDirectoryParams) middleware.Responder {
	scopedLog := log.With().
		Str("handler", "handleDeleteDirectory").
		Str("path", params.Path).
		Logger()

	if params.Path == "" {
		return ops_directories.NewDeleteDirectoryBadRequest().
			WithPayload(newAPIError(http.StatusBadRequest, WithMessage("Directory path cannot be empty")))
	}

	fi, err := os.Stat(params.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ops_directories.NewDeleteDirectoryNoContent()
		}

		scopedLog.Error().Err(err).Msg("Failed to stat directory")
		return ops_directories.NewDeleteDirectoryInternalServerError().
			WithPayload(newAPIError(http.StatusInternalServerError, WithMessage("Failed to stat directory")))
	}

	ifMatch := params.HTTPRequest.Header.Get("If-Match")
	if ifMatch == "" {
		return ops_directories.NewDeleteDirectoryPreconditionRequired().
			WithPayload(newAPIError(http.StatusPreconditionRequired, WithMessage("Missing If-Match header")))
	}

	currentETag := generateFileETag(fi)
	if ifMatch != currentETag {
		return ops_directories.NewDeleteDirectoryConflict().
			WithPayload(newAPIError(http.StatusConflict, WithMessage("ETag mismatch")))
	}

	err = os.RemoveAll(params.Path)
	switch {
	case err == nil:
	case errors.Is(err, os.ErrPermission):
		return ops_directories.NewDeleteDirectoryForbidden().
			WithPayload(newAPIError(http.StatusForbidden, WithMessage("Permission denied")))
	case errors.Is(err, os.ErrNotExist):
		// For some reason the directory was deleted between the last os.Stat call just
		// catch the error so it doesn't get caught by default
	default:
		scopedLog.Error().Err(err).Msg("Failed to delete directory")
		return ops_directories.NewDeleteDirectoryInternalServerError().
			WithPayload(newAPIError(http.StatusInternalServerError, WithMessage("Failed to delete directory")))
	}

	return ops_directories.NewDeleteDirectoryNoContent()
}

func putDirectory(path string, mode *os.FileMode, uid, gid *int) (created bool, err error) {
	fi, err := os.Stat(path)
	directoryExists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, newOpError(http.StatusInternalServerError, "Failed to stat directory", err)
	}

	if !directoryExists {
		// Create directory with all parent directories
		if err := os.MkdirAll(path, 0755); err != nil {
			return false, newOpError(http.StatusInternalServerError, "Failed to create directory", err)
		}
		created = true

		fi, err = os.Stat(path)
		if err != nil {
			return false, newOpError(http.StatusInternalServerError, "Failed to stat directory after creation", err)
		}
	} else if !fi.IsDir() {
		return false, newOpError(http.StatusBadRequest, "Path exists but is not a directory", nil)
	}

	stat := fi.Sys().(*syscall.Stat_t)
	currentMode := fi.Mode().Perm()
	currentUID := int(stat.Uid)
	currentGID := int(stat.Gid)

	if mode != nil && *mode != currentMode {
		if err := os.Chmod(path, *mode); err != nil {
			return created, newOpError(http.StatusInternalServerError, "Failed to chmod directory", err)
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
			return created, newOpError(http.StatusInternalServerError, "Failed to chown directory", err)
		}
	}

	return created, nil
}
