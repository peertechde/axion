package api

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/go-openapi/analysis"
	"github.com/go-openapi/loads"
	"github.com/rs/zerolog/hlog"
	"github.com/rs/zerolog/log"

	"peertech.de/axion/api/models"
	"peertech.de/axion/api/restapi"
	"peertech.de/axion/api/restapi/operations"
	ops_command "peertech.de/axion/api/restapi/operations/command"
	ops_content "peertech.de/axion/api/restapi/operations/content"
	ops_directories "peertech.de/axion/api/restapi/operations/directories"
	ops_files "peertech.de/axion/api/restapi/operations/files"
)

func New(opts ...Option) *API {
	var options Options
	for _, opt := range opts {
		opt(&options)
	}

	return &API{options: options}
}

type API struct {
	options    Options
	httpServer *http.Server
}

func (a *API) Initialize() error {
	if a.options.ListenAddr == "" {
		return fmt.Errorf("missing listen addr")
	}

	swaggerSpec, _, err := getSwaggerSpec()
	if err != nil {
		return err
	}

	openAPI := operations.NewConfigurationManagementAPI(swaggerSpec)
	openAPI.ServeError = serveError

	// Content
	openAPI.ContentDownloadHandler = ops_content.DownloadHandlerFunc(a.handleDownload)
	openAPI.ContentUploadHandler = ops_content.UploadHandlerFunc(a.handleUpload)

	// Files
	openAPI.CommandExecuteCommandHandler = ops_command.ExecuteCommandHandlerFunc(a.handleCommand)

	// Files
	openAPI.FilesGetFilePropertiesHandler = ops_files.GetFilePropertiesHandlerFunc(a.handleGetFileProperties)
	openAPI.FilesPutFileHandler = ops_files.PutFileHandlerFunc(a.handlePutFile)
	openAPI.FilesDeleteFileHandler = ops_files.DeleteFileHandlerFunc(a.handleDeleteFile)

	// Directories
	openAPI.DirectoriesGetDirectoryPropertiesHandler = ops_directories.GetDirectoryPropertiesHandlerFunc(a.handleGetDirectoryProperties)
	openAPI.DirectoriesPutDirectoryHandler = ops_directories.PutDirectoryHandlerFunc(a.handlePutDirectory)
	openAPI.DirectoriesDeleteDirectoryHandler = ops_directories.DeleteDirectoryHandlerFunc(a.handleDeleteDirectory)

	// Initialize the mux
	mux := http.NewServeMux()
	mux.Handle("/health", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	}))
	mux.Handle("/api/v1/", requestLogger(openAPI.Serve(nil)))

	a.httpServer = &http.Server{
		Handler:      mux,
		ReadTimeout:  a.options.ReadTimeout,
		IdleTimeout:  a.options.IdleTimeout,
		WriteTimeout: a.options.WriteTimeout,
		// TODO: ErrorLogger
		//ErrorLog:
	}

	return nil
}

// ServeError implements the http error handler interface
func serveError(rw http.ResponseWriter, r *http.Request, err error) {
	rw.Header().Set("Content-Type", "application/json")

	rw.WriteHeader(http.StatusInternalServerError)
	if r == nil || r.Method != http.MethodHead {
		_, _ = rw.Write(errorAsJSON(err.Error()))
	}

}

func errorAsJSON(err string) []byte {
	b, _ := json.Marshal(struct {
		Code    int64  `json:"code"`
		Message string `json:"message"`
	}{http.StatusInternalServerError, err})
	return b
}

func (a *API) listener() (net.Listener, error) {
	var ln net.Listener
	var err error

	if a.options.ServerTLSConfig == nil {
		ln, err = net.Listen("tcp", a.options.ListenAddr)
	} else {
		log.Info().Msg("Utilizing TLS...")
		ln, err = tls.Listen("tcp", a.options.ListenAddr, a.options.ServerTLSConfig)
	}

	return ln, err
}

func (a *API) Serve() error {
	ln, err := a.listener()
	if err != nil {
		return fmt.Errorf("failed to bind listener: %w", err)
	}
	defer ln.Close()

	return a.httpServer.Serve(ln)
}

func (a *API) Stop() error {
	stopctx, cancel := context.WithTimeout(context.Background(), a.options.GracefulTimeout)
	defer cancel()

	return a.httpServer.Shutdown(stopctx)
}

func getSwaggerSpec() (*loads.Document, *analysis.Spec, error) {
	// Load embedded swagger file.
	swaggerSpec, err := loads.Analyzed(restapi.SwaggerJSON, "")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load embedded swagger file: %w", err)
	}

	swaggerSpecAnalysis := analysis.New(swaggerSpec.Spec())
	return swaggerSpec, swaggerSpecAnalysis, nil
}

func requestLogger(next http.Handler) http.Handler {
	accessHandler := hlog.AccessHandler(
		func(r *http.Request, status, size int, duration time.Duration) {
			log.Info().
				Str("method", r.Method).
				Str("url", r.URL.Path).
				Str("proto", r.Proto).
				Str("raddr", r.RemoteAddr).
				Str("user-agent", r.UserAgent()).
				Int("status", status).
				Int("response_size_bytes", size).
				Str("duration", duration.String()).
				Msg("Handled request")
		},
	)
	return accessHandler(next)
}

type ErrorOption func(*ErrorOptions)

type ErrorOptions struct {
	Message string
	Details string
}

func WithMessage(msg string) ErrorOption {
	return func(o *ErrorOptions) {
		o.Message = msg
	}
}

func WithDetails(details string) ErrorOption {
	return func(o *ErrorOptions) {
		o.Details = details
	}
}

func newAPIError(code int, opts ...ErrorOption) *models.Error {
	var options ErrorOptions
	for _, opt := range opts {
		opt(&options)
	}
	return &models.Error{
		Code:    int64(code),
		Message: options.Message,
		Details: options.Details,
	}
}

type OpError struct {
	Code  int
	Msg   string
	Cause error
}

func (e *OpError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Msg, e.Cause)
	}
	return e.Msg
}

func (e *OpError) Unwrap() error {
	return e.Cause
}

func newOpError(code int, msg string, cause error) *OpError {
	return &OpError{
		Code:  code,
		Msg:   msg,
		Cause: cause,
	}
}
