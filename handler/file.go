package handler

import (
	"net/http"
	"os"
	"path"
	"strings"

	"go.avenga.cloud/couper/gateway/assets"
	"go.avenga.cloud/couper/gateway/utils"
)

const dirIndexFile = "index.html"

var (
	_ http.Handler = &File{}
	_ Lookupable   = &File{}
)

type Lookupable interface {
	HasResponse(req *http.Request) bool
}

type File struct {
	basePath string
	errAsset *assets.AssetFile
	rootDir  http.Dir
}

func NewFile(wd, basePath, docRoot string, asset *assets.AssetFile) *File {
	f := &File{
		basePath: basePath,
		errAsset: asset,
		rootDir:  http.Dir(path.Join(wd, docRoot)),
	}

	return f
}

func (f *File) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	reqPath := f.removeBasePath(req.URL.Path)

	file, info, err := f.openDocRootFile(reqPath)
	if err != nil {
		NewErrorHandler(f.errAsset, 3001, http.StatusNotFound).ServeHTTP(rw, req)
		return
	}
	defer file.Close()

	if info.IsDir() {
		f.serveDirectory(reqPath, rw, req)
		return
	}

	// TODO: gzip, br?
	http.ServeContent(rw, req, reqPath, info.ModTime(), file)
}

func (f *File) serveDirectory(reqPath string, rw http.ResponseWriter, req *http.Request) {
	if !strings.HasSuffix(reqPath, "/") {
		rw.Header().Set("Location", utils.JoinPath(req.URL.Path, "/"))
		rw.WriteHeader(http.StatusFound)
		return
	}

	reqPath = path.Join(reqPath, dirIndexFile)

	file, info, err := f.openDocRootFile(reqPath)
	if err != nil || info.IsDir() {
		NewErrorHandler(f.errAsset, 3001, http.StatusNotFound).ServeHTTP(rw, req)
		return
	}
	defer file.Close()

	// TODO: gzip, br?
	http.ServeContent(rw, req, reqPath, info.ModTime(), file)
}

func (f *File) HasResponse(req *http.Request) bool {
	reqPath := f.removeBasePath(req.URL.Path)

	file, _, err := f.openDocRootFile(reqPath)
	if err != nil {
		return false
	}
	defer file.Close()

	return true
}

func (f *File) openDocRootFile(name string) (http.File, os.FileInfo, error) {
	file, err := f.rootDir.Open(name)
	if err != nil {
		return nil, nil, err
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, err
	}

	return file, info, nil
}

func (f *File) removeBasePath(reqPath string) string {
	if strings.HasPrefix(reqPath, f.basePath) {
		return utils.JoinPath("/", reqPath[len(f.basePath):])
	} else if f.basePath != "/" {
		base := strings.TrimRight(f.basePath, "/")

		if strings.HasPrefix(reqPath, base) {
			return utils.JoinPath("/", reqPath[len(base):])
		}
	}

	return reqPath
}

func (f *File) String() string {
	return "File"
}
