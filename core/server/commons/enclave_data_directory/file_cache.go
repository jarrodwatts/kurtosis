/*
 * Copyright (c) 2021 - present Kurtosis Technologies Inc.
 * All Rights Reserved.
 */

package enclave_data_directory

import (
	"github.com/kurtosis-tech/stacktrace"
	"os"
	"path"
	"sync"
	"io"
	"github.com/sirupsen/logrus"
	"fmt"
)

// Represents a write-only file cache, backed by a directory inside the enclave data dir
type FileCache struct {
	absoluteDirpath              string
	dirpathRelativeToDataDirRoot string

	// Mutex to ensure we don't get race conditions when adding/getting files from the cache
	mutex *sync.Mutex
}

func newFileCache(absoluteDirpath string, dirpathRelativeToDataDirRoot string) *FileCache {
	return &FileCache{
		absoluteDirpath:              absoluteDirpath,
		dirpathRelativeToDataDirRoot: dirpathRelativeToDataDirRoot,
		mutex:                        &sync.Mutex{},
	}
}

func (cache *FileCache) AddFile(key string, reader io.Reader) (*EnclaveDataDirFile, error) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()

	newFileObj := cache.getFileObjFromKey(key)
	destAbsFilepath := newFileObj.absoluteFilepath
	if _, err := os.Stat(destAbsFilepath); err == nil {
		return nil, stacktrace.NewError("Cannot add file with key '%v' to the cache; a file with that key already exists", key)
	}

	functionCompletedSuccessfully := false
	fp, err := os.Create(destAbsFilepath)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred opening the filepointer of the new file with key '%v' being added to the cache", key)
	}

	defer func() {
		if !functionCompletedSuccessfully {
			if err := os.Remove(destAbsFilepath); err != nil {
				logrus.Errorf(
					"We encountered an error adding file with key '%v' to the cache so we tried to remove "+
						"the file we created, but got an error removing it:",
					key,
				)
				fmt.Fprintln(logrus.StandardLogger().Out, err)
				logrus.Errorf("!!!! ATTENTION !!!!! This means that the key '%v' will be corrupted!!", key)
			}
		}
	}()
	defer fp.Close()

	bytesLength, err := io.Copy(fp, reader)
	if err != nil {
		return nil, stacktrace.Propagate(err, "Writing could not be completed. Stopped writing at %v bytes.", bytesLength)
	}

	functionCompletedSuccessfully = true
	return newFileObj, nil
}

func (cache *FileCache) GetFile(key string) (*EnclaveDataDirFile, error) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()

	fileObj := cache.getFileObjFromKey(key)
	if _, err := os.Stat(fileObj.absoluteFilepath); os.IsNotExist(err) {
		return nil, stacktrace.NewError("No file with key '%v' exists in the cache", key)
	}

	return fileObj, nil
}

func (cache *FileCache) getFileObjFromKey(key string) *EnclaveDataDirFile {
	absoluteFilepath := path.Join(cache.absoluteDirpath, key)
	relativeFilepath := path.Join(cache.dirpathRelativeToDataDirRoot, key)
	return newEnclaveDataDirFile(absoluteFilepath, relativeFilepath)
}
