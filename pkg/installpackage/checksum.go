package installpackage

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/aquaproj/aqua/pkg/checksum"
	"github.com/aquaproj/aqua/pkg/config"
	"github.com/sirupsen/logrus"
	"github.com/spf13/afero"
	"github.com/suzuki-shunsuke/logrus-error/logerr"
)

func (inst *Installer) verifyChecksum(ctx context.Context, logE *logrus.Entry, checksums *checksum.Checksums, pkg *config.Package, assetName string, body io.Reader) (io.ReadCloser, error) { //nolint:cyclop,funlen,gocognit
	pkgInfo := pkg.PackageInfo
	tempDir, err := afero.TempDir(inst.fs, "", "")
	if err != nil {
		return nil, fmt.Errorf("create a temporal directory: %w", err)
	}
	defer inst.fs.RemoveAll(tempDir) //nolint:errcheck
	tempFilePath := filepath.Join(tempDir, assetName)
	if assetName == "" && (pkgInfo.Type == "github_archive" || pkgInfo.Type == "go") {
		tempFilePath = filepath.Join(tempDir, "archive.tar.gz")
	}
	file, err := inst.fs.Create(tempFilePath)
	if err != nil {
		return nil, fmt.Errorf("create a temporal file: %w", logerr.WithFields(err, logrus.Fields{
			"temp_file": tempFilePath,
		}))
	}
	defer file.Close()
	if _, err := io.Copy(file, body); err != nil {
		return nil, err //nolint:wrapcheck
	}
	calculatedSum, err := checksum.Calculate(inst.fs, tempFilePath, pkg.PackageInfo.Checksum.GetAlgorithm())
	if err != nil {
		return nil, fmt.Errorf("calculate a checksum of downloaded file: %w", logerr.WithFields(err, logrus.Fields{
			"temp_file": tempFilePath,
		}))
	}

	checksumID, err := pkg.GetChecksumID(inst.runtime)
	if err != nil {
		return nil, err //nolint:wrapcheck
	}
	chksum := checksums.Get(checksumID)

	if chksum == "" && pkgInfo.Checksum.Enabled() { //nolint:nestif
		logE.Info("downloading a checksum file")
		file, _, err := inst.checksumDownloader.DownloadChecksum(ctx, logE, inst.runtime, pkg)
		if err != nil {
			return nil, fmt.Errorf("download a checksum file: %w", err)
		}
		defer file.Close()
		b, err := io.ReadAll(file)
		if err != nil {
			return nil, fmt.Errorf("read a checksum file: %w", err)
		}
		m, err := inst.checksumFileParser.ParseChecksumFile(string(b), pkg)
		if err != nil {
			return nil, fmt.Errorf("parse a checksum file: %w", err)
		}
		for fileName, chksum := range m {
			chksumID, err := pkg.GetChecksumIDFromAsset(inst.runtime, fileName)
			if err != nil {
				logE.WithError(err).WithFields(logrus.Fields{
					"asset": fileName,
				}).Error("get checksum ID")
				continue
			}
			checksums.Set(chksumID, chksum)
		}
		c, ok := m[assetName]
		if ok {
			chksum = c
		}
	}

	if chksum != "" && calculatedSum != chksum {
		return nil, logerr.WithFields(errInvalidChecksum, logrus.Fields{ //nolint:wrapcheck
			"actual_checksum":   calculatedSum,
			"expected_checksum": chksum,
		})
	}
	if chksum == "" {
		checksums.Set(checksumID, calculatedSum)
	}
	readFile, err := inst.fs.Open(tempFilePath)
	if err != nil {
		return nil, err //nolint:wrapcheck
	}
	return readFile, nil
}
