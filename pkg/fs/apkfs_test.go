package fs

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadAPKFile(t *testing.T) {
	t.Run("stat", func(t *testing.T) {
		apkfs, err := NewAPKFS(context.Background(), "testdata/hello-2.12-r0.apk", APKFSPackage)
		require.Nil(t, err)
		defer apkfs.Close()
		require.NotNil(t, apkfs)
		file, err := apkfs.Open("/usr/bin/hello")
		require.Nil(t, err)
		defer file.Close()
		info, err := file.Stat()
		require.Nil(t, err)
		require.Equal(t, info.Name(), "hello")
	})
	t.Run("read", func(t *testing.T) {
		apkfs, err := NewAPKFS(context.Background(), "testdata/hello-2.12-r0.apk", APKFSPackage)
		require.Nil(t, err)
		defer apkfs.Close()
		require.NotNil(t, apkfs)
		file, err := apkfs.Open("/usr/bin/hello")
		require.Nil(t, err)
		defer file.Close()
		info, err := file.Stat()
		require.Nil(t, err)
		buffer := make([]byte, 4096)
		var readSoFar int64
		for {
			readThisTime, err := file.Read(buffer)
			if err != io.EOF {
				require.Nil(t, err)
			}
			readSoFar += int64(readThisTime)
			if readThisTime == 0 {
				break
			}
		}
		require.Equal(t, info.Size(), readSoFar)
		require.Equal(t, info.Name(), "hello")
	})
}
