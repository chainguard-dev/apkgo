// Copyright 2023 Chainguard, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package signature

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/gzip"

	"github.com/psanford/memfs"

	"github.com/chainguard-dev/go-apk/pkg/tarball"
)

func SignIndex(ctx context.Context, signingKey string, indexFile string) error {
	is, err := indexIsAlreadySigned(indexFile)
	if err != nil {
		return err
	}
	if is {
		log.Printf("index %s is already signed, doing nothing", indexFile)
		return nil
	}

	log.Printf("signing index %s with key %s", indexFile, signingKey)

	// Unfortunately apk-tools checks signatures until the first one passes, and skips the rest.
	// And golang sorts its reads & writes in lexical order only. Thus indexes will be
	// validated with SHA1 only by apk-tools whilst RSA (SHA1) signature is present.
	//
	// An incremental WriteTargz would allow to write out strongest hash first. Or a MergeFS
	// implementation the maintains relative order.
	//
	// Step 0) apk-tools supports RSA256 since 2017
	// Step 1) Upgrade all deployments of melange/go-apk with verification support for RSA256
	// Step 2) Turn off RSA signatures & turn on RSA256 signatures
	//
	// Enable both (incorrectly ordered dual-signed) only for local testing
	sigs := []struct {
		filename   string
		digestType crypto.Hash
	}{
		{
			"RSA",
			crypto.SHA1,
		},
		// {
		// 	"RSA256",
		// 	crypto.SHA256,
		// },
	}

	indexData, err := os.ReadFile(indexFile)
	if err != nil {
		return fmt.Errorf("unable to read index for signing: %w", err)
	}

	sigFS := memfs.New()

	for _, sig := range sigs {
		indexDigest, err := HashData(indexData, sig.digestType)
		if err != nil {
			return err
		}

		sigData, err := RSASignDigest(indexDigest, sig.digestType, signingKey, "")
		if err != nil {
			return fmt.Errorf("unable to sign index: %w", err)
		}

		log.Printf("appending signature %s to index %s", sig.filename, indexFile)

		if err := sigFS.WriteFile(fmt.Sprintf(".SIGN.%s.%s.pub", sig.filename, filepath.Base(signingKey)), sigData, 0644); err != nil {
			return fmt.Errorf("unable to append signature: %w", err)
		}
	}

	// prepare control.tar.gz
	multitarctx, err := tarball.NewContext(
		tarball.WithOverrideUIDGID(0, 0),
		tarball.WithOverrideUname("root"),
		tarball.WithOverrideGname("root"),
		tarball.WithSkipClose(true),
	)
	if err != nil {
		return fmt.Errorf("unable to build tarball context: %w", err)
	}

	log.Printf("writing signed index to %s", indexFile)

	var sigBuffer bytes.Buffer
	if err := multitarctx.WriteTargz(ctx, &sigBuffer, sigFS, sigFS); err != nil {
		return fmt.Errorf("unable to write signature tarball: %w", err)
	}

	idx, err := os.Create(indexFile)
	if err != nil {
		return fmt.Errorf("unable to open index for writing: %w", err)
	}
	defer idx.Close()

	if _, err := io.Copy(idx, &sigBuffer); err != nil {
		return fmt.Errorf("unable to write index signature: %w", err)
	}

	if _, err := idx.Write(indexData); err != nil {
		return fmt.Errorf("unable to write index data: %w", err)
	}

	log.Printf("signed index %s with key %s", indexFile, signingKey)

	return nil
}

func indexIsAlreadySigned(indexFile string) (bool, error) {
	index, err := os.Open(indexFile)
	if err != nil {
		return false, fmt.Errorf("cannot open index file %s: %w", indexFile, err)
	}
	defer index.Close()

	gzi, err := gzip.NewReader(index)
	if err != nil {
		return false, fmt.Errorf("cannot open index file %s as gzip: %w", indexFile, err)
	}
	defer gzi.Close()

	tari := tar.NewReader(gzi)
	for {
		hdr, err := tari.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return false, fmt.Errorf("cannot read tar index %s: %w", indexFile, err)
		}

		if strings.HasPrefix(hdr.Name, ".SIGN.RSA") {
			return true, nil
		}
	}

	return false, nil
}

func HashData(data []byte, digestType crypto.Hash) ([]byte, error) {
	digest := digestType.New()
	if n, err := digest.Write(data); err != nil || n != len(data) {
		return nil, fmt.Errorf("unable to hash data: %w", err)
	}
	return digest.Sum(nil), nil
}
