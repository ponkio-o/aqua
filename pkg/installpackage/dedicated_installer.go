package installpackage

import (
	"context"
	"fmt"
	"sync"

	"github.com/aquaproj/aqua/v2/pkg/checksum"
	"github.com/aquaproj/aqua/v2/pkg/config"
	"github.com/sirupsen/logrus"
)

type DedicatedInstaller struct {
	installer *Installer
	mutex     *sync.Mutex
	pkg       func() *config.Package
	checksums map[string]string
}

func newDedicatedInstaller(installer *Installer, pkg func() *config.Package, checksums map[string]string) *DedicatedInstaller {
	return &DedicatedInstaller{
		installer: installer,
		mutex:     &sync.Mutex{},
		pkg:       pkg,
		checksums: checksums,
	}
}

func (di *DedicatedInstaller) install(ctx context.Context, logE *logrus.Entry) error {
	di.mutex.Lock()
	defer di.mutex.Unlock()

	pkg := di.pkg()
	logE = logE.WithFields(logrus.Fields{
		"package_name":    pkg.Package.Name,
		"package_version": pkg.Package.Version,
	})

	pkgInfo, err := pkg.PackageInfo.Override(logE, pkg.Package.Version, di.installer.runtime)
	if err != nil {
		return fmt.Errorf("evaluate version constraints: %w", err)
	}
	supported, err := pkgInfo.CheckSupported(di.installer.runtime, di.installer.runtime.Env())
	if err != nil {
		return fmt.Errorf("check if the package is supported in the environment: %w", err)
	}
	if !supported {
		logE.Debug("the package isn't supported in the environment")
		return nil
	}

	pkg.PackageInfo = pkgInfo
	chksum := di.checksums[fmt.Sprintf("%s/%s", di.installer.runtime.GOOS, di.installer.runtime.Arch(pkg.PackageInfo.Rosetta2, pkg.PackageInfo.WindowsARMEmulation))]

	if err := di.installer.InstallPackage(ctx, logE, &ParamInstallPackage{
		Checksums: checksum.New(), // Check checksum but not update aqua-checksums.json
		Pkg:       pkg,
		Checksum: &checksum.Checksum{
			Algorithm: "sha256",
			Checksum:  chksum,
		},
		DisablePolicy: true,
	}); err != nil {
		return err
	}

	return nil
}